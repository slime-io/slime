package controllers

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	prometheusApi "github.com/prometheus/client_golang/api"
	prometheusV1 "github.com/prometheus/client_golang/api/prometheus/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"

	"slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/trigger"
	"slime.io/slime/framework/util"
)

// StaticMeta is static info and do not to query from prometheus
type StaticMeta struct {
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	NPod      map[string]int  `json:"nPod"`
	IsGroup   map[string]bool `json:"isGroup"`
}

func (s StaticMeta) String() string {
	b, err := json.Marshal(s)
	if err != nil {
		log.Errorf("marshal meta err :%v", err.Error())
		return ""
	}
	return string(b)
}

// the following functions is registered to framework

// handleWatcherEvent is triggered by endpoint event
func (r *SmartLimiterReconciler) handleWatcherEvent(event trigger.WatcherEvent) metric.QueryMap {
	queryMap := make(map[string][]metric.Handler, 0)
	log.Infof("%v trigger handleWatcherEvent", event)
	_, ok := r.interest.Get(FQN(event.NN.Namespace, event.NN.Name))
	if !ok {
		log.Debugf("key %s not in interest map", event.NN)
		return queryMap
	}
	return r.handleEvent(event.NN)
}

// handleTickerEvent is triggered by ticker
func (r *SmartLimiterReconciler) handleTickerEvent(event trigger.TickerEvent) metric.QueryMap {
	log.Debugf("ticker trigger handleTickerEvent")
	queryMap := make(map[string][]metric.Handler, 0)
	// traverse interest map, if gw, skip
	for k := range r.interest.Items() {
		item := strings.Split(k, "/")
		namespace, name := item[0], item[1]
		qm := r.handleEvent(types.NamespacedName{Namespace: namespace, Name: name})

		for meta, handlers := range qm {
			queryMap[meta] = handlers
		}
	}
	return queryMap
}

func (r *SmartLimiterReconciler) handleEvent(loc types.NamespacedName) metric.QueryMap {
	queryMap := make(map[string][]metric.Handler, 0)
	meta, ok := r.interest.Get(FQN(loc.Namespace, loc.Name))
	if !ok {
		log.Warnf("%s not in interest map", loc)
		return queryMap
	}

	// handle loc which is in interest map in inbound scenario
	if !r.cfg.GetDisableAdaptive() {
		return r.handlePrometheusEvent(meta, loc)
	}
	// unify mesh and gateway in local event
	return r.handleLocalEvent(meta, loc)
}

func (r *SmartLimiterReconciler) handleLocalEvent(meta SmartLimiterMeta, loc types.NamespacedName) metric.QueryMap {
	queryMap := make(map[string][]metric.Handler, 0)

	if len(meta.workloadSelector) > 0 {
		// workloadSelector is specified
		queryMap = r.genQuerymapWithWorkloadSelector(meta, loc)
	} else if host := meta.seHost; host != "" {
		// se host is specified
		queryMap = r.genQuerymapWithServiceEntry(host, loc)
	} else {
		// no workloadSelector and se host

		// in gateway, workloadSelector can be not specified
		// just build querymap with empty handler
		if meta.gateway {
			sm := &StaticMeta{Name: loc.Name, Namespace: loc.Namespace}
			queryMap[sm.String()] = []metric.Handler{}
			return queryMap
		}
		// no workloadSelector and se host, not gateway, use k8s service
		queryMap = r.genQuerymapWithService(loc)
	}
	return queryMap
}

// handlePrometheusEvent means construct query map as following
// example: handler is a map
// cpu.max => max(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
func (r *SmartLimiterReconciler) handlePrometheusEvent(LimiterMeta SmartLimiterMeta, loc types.NamespacedName) metric.QueryMap {
	if r.env.Config == nil || r.env.Config.Metric == nil || r.env.Config.Metric.Prometheus == nil || r.env.Config.Metric.Prometheus.Handlers == nil {
		log.Infof("query handler is empty, skip query")
		return nil
	}
	handlers := r.env.Config.Metric.Prometheus.Handlers

	// TODO workloadSelector
	if len(LimiterMeta.workloadSelector) > 0 {
		log.Warnf("promql is closed when workloadSelector is specified in %s", loc)
		return nil
	} else if host := LimiterMeta.seHost; host != "" {
		log.Warnf("promql is closed when se host is specified in %s", loc)
		return nil
	}

	pods, err := queryServicePods(r.Client, loc)
	if err != nil {
		log.Infof("get err in queryServicePods, %+v", err.Error())
		return nil
	}

	subsetsPods, err := querySubsetPods(pods, loc)
	if err != nil {
		log.Infof("%+v", err.Error())
		return nil
	}
	return generateQueryString(subsetsPods, loc, handlers)
}

// QueryServicePods query pods related to service, return pods
func queryServicePods(c client.Client, loc types.NamespacedName) ([]v1.Pod, error) {
	var err error
	service := &v1.Service{}
	pods := make([]v1.Pod, 0)

	if err = c.Get(context.TODO(), loc, service); err != nil {
		return pods, fmt.Errorf("get service %+v faild, %s", loc, err.Error())
	}
	return queryPodsWithWorkloadSelector(c, service.Spec.Selector, loc.Namespace)
}

func queryPodsWithWorkloadSelector(c client.Client, workloadSelector map[string]string, ns string) ([]v1.Pod, error) {
	pods := make([]v1.Pod, 0)

	podLists := &v1.PodList{}
	if err := c.List(context.TODO(), podLists, client.InNamespace(ns), client.MatchingLabels(workloadSelector)); err != nil {
		return pods, fmt.Errorf("list pods with selector %+v failed", workloadSelector)
	}

	for _, item := range podLists.Items {
		if item.DeletionTimestamp != nil {
			// pod is deleted
			continue
		}
		if item.Status.Phase != v1.PodRunning {
			// pods not running
			log.Infof("pod %s/%s is not running. Status=%v. skip", item.Namespace, item.Name, item.Status.Phase)
			continue
		}
		pods = append(pods, item)
	}
	if len(pods) == 0 {
		return pods, fmt.Errorf("no pods match workloadSelector %+v", workloadSelector)
	}

	return pods, nil
}

// QuerySubsetPods  query pods related to subset
func querySubsetPods(pods []v1.Pod, loc types.NamespacedName) (map[string][]string, error) {
	subsetsPods := make(map[string][]string)
	host := util.UnityHost(loc.Name, loc.Namespace)

	// if subset is existed, assign pods to subset
	if subsets := controllers.HostSubsetMapping.Get(host); len(subsets) > 0 {
		for _, pod := range pods {
			for _, sb := range subsets {
				if util.IsContain(pod.Labels, sb.Labels) {
					append2Subsets(sb.GetName(), subsetsPods, pod)
				}
			}
		}
	}
	for _, pod := range pods {
		append2Base(subsetsPods, pod)
	}
	return subsetsPods, nil
}

func append2Subsets(subsetName string, subsetsPods map[string][]string, pod v1.Pod) {
	if subsetsPods[subsetName] != nil {
		subsetsPods[subsetName] = append(subsetsPods[subsetName], pod.Name)
	} else {
		subsetsPods[subsetName] = []string{pod.Name}
	}
}

func append2Base(subsetsPods map[string][]string, pod v1.Pod) {
	if subsetsPods[util.WellknownBaseSet] != nil {
		subsetsPods[util.WellknownBaseSet] = append(subsetsPods[util.WellknownBaseSet], pod.Name)
	} else {
		subsetsPods[util.WellknownBaseSet] = []string{pod.Name}
	}
}

// GenerateQueryString
func generateQueryString(subsetsPods map[string][]string, loc types.NamespacedName, handlers map[string]*v1alpha1.Prometheus_Source_Handler) map[string][]metric.Handler {
	queryMap := make(map[string][]metric.Handler, 0)
	queryHandlers := make([]metric.Handler, 0)
	isGroup := make(map[string]bool)

	m := make(map[string]int)
	for key, item := range subsetsPods {
		m[key] = len(item)
	}
	meta := generateMeta(m, loc)

	//  example
	//	item 	=>  cpu.max: max(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
	for customMetricName, handler := range handlers {
		if handler.Query == "" {
			continue
		}
		queryHandlers, isGroup = replaceQueryString(customMetricName, handler.Query, handler.Type, loc, subsetsPods)

		for name, group := range isGroup {
			meta.IsGroup[name] = group
		}
	}
	metaInfo := meta.String()
	if metaInfo == "" {
		return queryMap
	}
	queryMap[metaInfo] = append(queryMap[metaInfo], queryHandlers...)
	return queryMap
}

// some metric is not query from prometheus, so add it to staticMeta
func generateMeta(subsetsPods map[string]int, loc types.NamespacedName) StaticMeta {
	// NPOD record like
	// _base.pod: 6
	// v1.pod: 2
	// v2.pod: 4
	nPod := make(map[string]int)
	for k, v := range subsetsPods {
		if v > 0 {
			nPod[k+".pod"] = v
		}
	}
	meta := StaticMeta{
		Name:      loc.Name,
		Namespace: loc.Namespace,
		NPod:      nPod,
		IsGroup:   map[string]bool{},
	}
	return meta
}

func replaceQueryString(metricName string, query string, typ v1alpha1.Prometheus_Source_Type, loc types.NamespacedName, subsetsPods map[string][]string) ([]metric.Handler, map[string]bool) {
	handlers := make([]metric.Handler, 0)
	isGroup := make(map[string]bool)
	query = strings.ReplaceAll(query, "$namespace", loc.Namespace)

	switch typ {
	case v1alpha1.Prometheus_Source_Value:
		if strings.Contains(query, "$pod_name") {
			// each subset will generate a query
			for subsetName, subsetPods := range subsetsPods {
				subQuery := strings.ReplaceAll(query, "$pod_name", strings.Join(subsetPods, "|"))

				h := metric.Handler{
					Name:  subsetName + "." + metricName,
					Query: subQuery,
				}
				// handlers hold all query and real metricName
				handlers = append(handlers, h)
				isGroup[h.Name] = false
			}
		} else {
			h := metric.Handler{
				Name:  metricName,
				Query: query,
			}
			handlers = append(handlers, h)
			isGroup[h.Name] = false
		}
	case v1alpha1.Prometheus_Source_Group:
		h := metric.Handler{
			Name:  metricName,
			Query: query,
		}
		handlers = append(handlers, h)
		isGroup[h.Name] = true
	}
	return handlers, isGroup
}

func newPrometheusSourceConfig(env bootstrap.Environment) (metric.PrometheusSourceConfig, error) {
	if env.Config == nil || env.Config.Metric == nil || env.Config.Metric.Prometheus == nil {
		return metric.PrometheusSourceConfig{}, stderrors.New("failure create prometheus client, empty prometheus config")
	}
	promClient, err := prometheusApi.NewClient(prometheusApi.Config{
		Address:      env.Config.Metric.Prometheus.Address,
		RoundTripper: nil,
	})
	if err != nil {
		return metric.PrometheusSourceConfig{}, err
	}
	return metric.PrometheusSourceConfig{
		Api: prometheusV1.NewAPI(promClient),
	}, nil
}

func (r *SmartLimiterReconciler) genQuerymapWithWorkloadSelector(LimiterMeta SmartLimiterMeta, loc types.NamespacedName) map[string][]metric.Handler {
	// here, there is such a semantics
	// if it is under istioNs, it will match all the pods in cluster
	// other it will only match the real ns in cluster
	queryMap := make(map[string][]metric.Handler, 0)
	ns := loc.Namespace
	if loc.Namespace == r.env.Config.Global.IstioNamespace {
		ns = ""
	}
	pods, err := queryPodsWithWorkloadSelector(r.Client, LimiterMeta.workloadSelector, ns)
	if err != nil {
		log.Errorf("get err in queryServicePodsWithWorkloadSelector, %+v", err.Error())
		return nil
	}

	subsetInfo := make(map[string]int)
	subsetInfo[util.WellknownBaseSet] = len(pods)

	meta := generateMeta(subsetInfo, loc)
	metaInfo := meta.String()
	log.Debugf("get workloadSelector meta info %s", metaInfo)
	if metaInfo == "" {
		return nil
	}
	queryMap[metaInfo] = []metric.Handler{}
	return queryMap
}

func (r *SmartLimiterReconciler) genQuerymapWithServiceEntry(host string, loc types.NamespacedName) map[string][]metric.Handler {
	queryMap := make(map[string][]metric.Handler, 0)

	svc, err := getIstioService(r, types.NamespacedName{Namespace: loc.Namespace, Name: host})
	if err != nil {
		log.Errorf("get empty istio service base on %s/%s, %s", loc.Namespace, host, err)
		return queryMap
	}
	serviceLabels := formatLabels(getIstioServiceLabels(svc))
	subsetInfo := make(map[string]int)
	subsetInfo[util.WellknownBaseSet] = len(svc.Endpoints)

	if subsets := controllers.HostSubsetMapping.Get(host); len(subsets) > 0 {
		for _, ep := range svc.Endpoints {
			for _, sb := range subsets {
				if util.IsContain(ep.Labels, serviceLabels) {
					subsetInfo[sb.GetName()] = subsetInfo[sb.GetName()] + 1
				}
			}
		}
	}
	meta := generateMeta(subsetInfo, loc)
	metaInfo := meta.String()
	log.Debugf("get se meta info %s", metaInfo)
	if metaInfo == "" {
		return nil
	}
	queryMap[metaInfo] = []metric.Handler{}
	return queryMap
}

func (r *SmartLimiterReconciler) genQuerymapWithService(loc types.NamespacedName) map[string][]metric.Handler {
	// otherwise, use k8s svc
	queryMap := make(map[string][]metric.Handler, 0)
	pods, err := queryServicePods(r.Client, loc)
	if err != nil {
		log.Infof("get err in queryServicePods, %+v", err.Error())
		return nil
	}

	subsetsPods, err := querySubsetPods(pods, loc)
	if err != nil {
		log.Infof("%+v", err.Error())
		return nil
	}
	sbInfo := make(map[string]int)
	for key, item := range subsetsPods {
		sbInfo[key] = len(item)
	}
	meta := generateMeta(sbInfo, loc)
	metaInfo := meta.String()
	if metaInfo == "" {
		return nil
	}
	queryMap[metaInfo] = []metric.Handler{}
	return queryMap
}
