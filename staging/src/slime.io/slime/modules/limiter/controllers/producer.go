package controllers

import (
	"context"
	stderrors "errors"
	"fmt"
	"strings"

	prometheusApi "github.com/prometheus/client_golang/api"
	prometheusV1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	"slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/trigger"
	"slime.io/slime/framework/util"
	"slime.io/slime/modules/limiter/model"
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

// TODO serviceEntry
func (r *SmartLimiterReconciler) handleEvent(loc types.NamespacedName) metric.QueryMap {
	queryMap := make(map[string][]metric.Handler, 0)
	v, ok := r.interest.Get(FQN(loc.Namespace, loc.Name))
	if !ok {
		log.Infof("%s not in interest map", loc)
		return queryMap
	}
	// no need to query service information in outbound and gateway scenario
	host := v.(string)
	if host == model.MockHost {
		meta := &StaticMeta{Name: loc.Name, Namespace: loc.Namespace}
		metaInfo := meta.String()
		queryMap[metaInfo] = []metric.Handler{}
		return queryMap
	}

	// handle loc which is in interest map in inbound scenario
	if !r.cfg.GetDisableAdaptive() {
		return r.handlePrometheusEvent(host, loc)
	} else {
		return r.handleLocalEvent(host, loc)
	}
	return nil
}

func (r *SmartLimiterReconciler) handleLocalEvent(host string, loc types.NamespacedName) metric.QueryMap {
	queryMap := make(map[string][]metric.Handler, 0)

	// if it does not end with .svc.cluster.local, it means that se host is specified
	if !strings.HasSuffix(host, util.WellknownK8sSuffix) {
		svc, err := getIstioService(r, types.NamespacedName{Namespace: loc.Namespace, Name: host})
		if err != nil {
			log.Errorf("get empty istio service base on %s/%s, %s", loc.Namespace, host, err)
			return queryMap
		}
		serviceLabels := formatLabels(getIstioServiceLabels(svc))
		subsetInfo := make(map[string]int)
		// if subset is existed, assign pods to subset
		subsetInfo[util.WellknownBaseSet] = len(svc.Endpoints)

		if controllers.HostSubsetMapping.Get(host) != nil {
			if subsets, ok := controllers.HostSubsetMapping.Get(host).([]*v1alpha3.Subset); ok {
				for _, ep := range svc.Endpoints {
					for _, sb := range subsets {
						if util.IsContain(ep.Labels, serviceLabels) {
							subsetInfo[sb.GetName()] = subsetInfo[sb.GetName()] + 1
						}
					}
				}
			}
		}
		meta := generateMeta(subsetInfo, loc)
		metaInfo := meta.String()
		log.Infof("get se meta info %s", metaInfo)
		if metaInfo == "" {
			return nil
		}
		queryMap[metaInfo] = []metric.Handler{}
		return queryMap
	}

	// otherwise, use k8s svc
	pods, err := queryServicePods(r.env.K8SClient, loc)
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

// handlePrometheusEvent means construct query map as following
// example: handler is a map
// cpu.max => max(container_cpu_usage_seconds_total{namespace="$namespace",pod=~"$pod_name",image=""})
func (r *SmartLimiterReconciler) handlePrometheusEvent(host string, loc types.NamespacedName) metric.QueryMap {
	if r.env.Config == nil || r.env.Config.Metric == nil || r.env.Config.Metric.Prometheus == nil || r.env.Config.Metric.Prometheus.Handlers == nil {
		log.Infof("query handler is empty, skip query")
		return nil
	}
	handlers := r.env.Config.Metric.Prometheus.Handlers

	if !strings.HasSuffix(host, util.WellknownK8sSuffix) {
		log.Errorf("promql is closed when se host is specified")
		return nil
	}

	pods, err := queryServicePods(r.env.K8SClient, loc)
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
func queryServicePods(c *kubernetes.Clientset, loc types.NamespacedName) ([]v1.Pod, error) {
	var err error
	var service *v1.Service
	pods := make([]v1.Pod, 0)

	service, err = c.CoreV1().Services(loc.Namespace).Get(context.TODO(), loc.Name, metav1.GetOptions{})
	if err != nil {
		return pods, fmt.Errorf("get service %+v faild, %s", loc, err.Error())
	}
	podList, err := c.CoreV1().Pods(loc.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
	})
	if err != nil {
		return pods, fmt.Errorf("query pod list faild, %+v", err.Error())
	}

	for _, item := range podList.Items {
		if item.DeletionTimestamp != nil {
			// pod is deleted
			continue
		}
		if item.Status.Phase != v1.PodRunning {
			// pods not running
			log.Debugf("pod %s/%s is not running. Status=%v. skip", item.Namespace, item.Name, item.Status.Phase)
			continue
		}
		pods = append(pods, item)
	}
	return pods, nil
}

// QuerySubsetPods  query pods related to subset
func querySubsetPods(pods []v1.Pod, loc types.NamespacedName) (map[string][]string, error) {
	subsetsPods := make(map[string][]string)
	host := util.UnityHost(loc.Name, loc.Namespace)

	// if subset is existed, assign pods to subset
	if controllers.HostSubsetMapping.Get(host) != nil {
		subsets, ok := controllers.HostSubsetMapping.Get(host).([]*v1alpha3.Subset)
		if ok {
			for _, pod := range pods {
				for _, sb := range subsets {
					if util.IsContain(pod.Labels, sb.Labels) {
						append2Subsets(sb.GetName(), subsetsPods, pod)
					}
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
