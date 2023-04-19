package controllers

import (
	"context"
	stderrors "errors"
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"strconv"
	"strings"
	"time"

	envoy_config_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	data_accesslog "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	prometheusApi "github.com/prometheus/client_golang/api"
	prometheusV1 "github.com/prometheus/client_golang/api/prometheus/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/trigger"
	lazyloadapiv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
)

const (
	AccessLogConvertorName     = "lazyload-accesslog-convertor"
	MetricSourceTypePrometheus = "prometheus"
	MetricSourceTypeAccesslog  = "accesslog"
	SvfResource                = "servicefences"
)

// call back function for watcher producer
func (r *ServicefenceReconciler) handleWatcherEvent(event trigger.WatcherEvent) metric.QueryMap {
	// check event
	gvks := []schema.GroupVersionKind{
		{Group: "networking.istio.io", Version: "v1alpha3", Kind: "Sidecar"},
	}
	invalidEvent := false
	for _, gvk := range gvks {
		if event.GVK == gvk && r.getInterestMeta()[event.NN.String()] {
			invalidEvent = true
		}
	}
	if !invalidEvent {
		return nil
	}

	// generate query map for producer
	qm := make(map[string][]metric.Handler)
	var hs []metric.Handler

	// check metric source type
	switch r.env.Config.Global.Misc["metricSourceType"] {
	case MetricSourceTypePrometheus:
		for pName, pHandler := range r.env.Config.Metric.Prometheus.Handlers {
			hs = append(hs, generateHandler(event.NN.Name, event.NN.Namespace, pName, pHandler))
		}
	case MetricSourceTypeAccesslog:
		hs = []metric.Handler{
			{
				Name:  AccessLogConvertorName,
				Query: "",
			},
		}
	}

	qm[event.NN.String()] = hs
	return qm
}

// call back function for ticker producer
func (r *ServicefenceReconciler) handleTickerEvent(event trigger.TickerEvent) metric.QueryMap {
	// no need to check time duration

	// generate query map for producer
	// check metric source type
	qm := make(map[string][]metric.Handler)

	switch r.env.Config.Global.Misc["metricSourceType"] {
	case MetricSourceTypePrometheus:
		for meta := range r.getInterestMeta() {
			namespace, name := strings.Split(meta, "/")[0], strings.Split(meta, "/")[1]
			var hs []metric.Handler
			for pName, pHandler := range r.env.Config.Metric.Prometheus.Handlers {
				hs = append(hs, generateHandler(name, namespace, pName, pHandler))
			}
			qm[meta] = hs
		}
	case MetricSourceTypeAccesslog:
		for meta := range r.getInterestMeta() {
			qm[meta] = []metric.Handler{
				{
					Name:  AccessLogConvertorName,
					Query: "",
				},
			}
		}
	}

	return qm
}

func generateHandler(name, namespace, pName string, pHandler *v1alpha1.Prometheus_Source_Handler) metric.Handler {
	query := strings.ReplaceAll(pHandler.Query, "$namespace", namespace)
	query = strings.ReplaceAll(query, "$source_app", name)
	return metric.Handler{Name: pName, Query: query}
}

func NewProducerConfig(env bootstrap.Environment) (*metric.ProducerConfig, error) {
	// init metric source
	var enablePrometheusSource bool
	var prometheusSourceConfig metric.PrometheusSourceConfig
	var accessLogSourceConfig metric.AccessLogSourceConfig
	var err error

	switch env.Config.Global.Misc["metricSourceType"] {
	case MetricSourceTypePrometheus:
		enablePrometheusSource = true
		prometheusSourceConfig, err = newPrometheusSourceConfig(env)
		if err != nil {
			return nil, err
		}
	case MetricSourceTypeAccesslog:
		enablePrometheusSource = false
		// init log source port
		port := env.Config.Global.Misc["logSourcePort"]

		// init accessLog source config
		accessLogSourceConfig = metric.AccessLogSourceConfig{
			ServePort: port,
			AccessLogConvertorConfigs: []metric.AccessLogConvertorConfig{
				{
					Name:    AccessLogConvertorName,
					Handler: nil,
				},
			},
		}
	default:
		return nil, stderrors.New("wrong metricSourceType")
	}

	// init whole producer config
	pc := &metric.ProducerConfig{
		EnablePrometheusSource: enablePrometheusSource,
		PrometheusSourceConfig: prometheusSourceConfig,
		AccessLogSourceConfig:  accessLogSourceConfig,
		EnableWatcherProducer:  true,
		WatcherProducerConfig: metric.WatcherProducerConfig{
			Name:       "lazyload-watcher",
			MetricChan: make(chan metric.Metric),
			WatcherTriggerConfig: trigger.WatcherTriggerConfig{
				Kinds: []schema.GroupVersionKind{
					{
						Group:   "networking.istio.io",
						Version: "v1alpha3",
						Kind:    "Sidecar",
					},
				},
				EventChan:     make(chan trigger.WatcherEvent),
				DynamicClient: env.DynamicClient,
			},
		},
		EnableTickerProducer: true,
		TickerProducerConfig: metric.TickerProducerConfig{
			Name:       "lazyload-ticker",
			MetricChan: make(chan metric.Metric),
			TickerTriggerConfig: trigger.TickerTriggerConfig{
				Durations: []time.Duration{
					10 * time.Second,
				},
				EventChan: make(chan trigger.TickerEvent),
			},
		},
		StopChan: env.Stop,
	}

	return pc, nil
}

func (r *ServicefenceReconciler) LogHandler(logEntry []*data_accesslog.HTTPAccessLogEntry) (map[string]map[string]string, error) {
	return accessLogHandler(logEntry, r.ipToSvcCache, r.svcToIpsCache, r.ipTofence, r.fenceToIp, r.cfg.EnableShortDomain)
}

func newPrometheusSourceConfig(env bootstrap.Environment) (metric.PrometheusSourceConfig, error) {
	ps := env.Config.Metric.Prometheus
	if ps == nil {
		return metric.PrometheusSourceConfig{}, stderrors.New("failure create prometheus client, empty prometheus config")
	}
	promClient, err := prometheusApi.NewClient(prometheusApi.Config{
		Address:      ps.Address,
		RoundTripper: nil,
	})
	if err != nil {
		return metric.PrometheusSourceConfig{}, err
	}

	return metric.PrometheusSourceConfig{
		Api: prometheusV1.NewAPI(promClient),
	}, nil
}

func NewCache(env bootstrap.Environment) (map[string]map[string]string, error) {
	result := make(map[string]map[string]string)

	svfGvr := schema.GroupVersionResource{
		Group:    lazyloadapiv1alpha1.GroupVersion.Group,
		Version:  lazyloadapiv1alpha1.GroupVersion.Version,
		Resource: SvfResource,
	}

	dc := env.DynamicClient
	svfList, err := dc.Resource(svfGvr).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list servicefence error: %v", err)
	}
	for _, svf := range svfList.Items {
		meta := svf.GetNamespace() + "/" + svf.GetName()
		value := make(map[string]string)
		ms, existed, err := unstructured.NestedMap(svf.Object, "status", "metricStatus")
		if err != nil {
			log.Errorf("got servicefence %s status.metricStatus error: %v", meta, err)
			continue
		}
		if existed {
			for k, v := range ms {
				ok := false
				if value[k], ok = v.(string); !ok {
					log.Errorf("servicefence %s metricStatus value is not string, value: %+v", meta, v)
					continue
				}
			}
		}
		result[meta] = value
	}

	return result, nil
}

func accessLogHandler(logEntry []*data_accesslog.HTTPAccessLogEntry, ipToSvcCache *IpToSvcCache,
	svcToIpsCache *SvcToIpsCache, ipTofenceCache *IpTofence, fenceToIpCache *FenceToIp, enableShortDomain bool,
) (map[string]map[string]string, error) {

	log = log.WithField("reporter", "accesslog convertor").WithField("function", "accessLogHandler")
	result := make(map[string]map[string]string)
	tmpResult := make(map[string]map[string]int)

	for _, entry := range logEntry {

		// fetch sourceEp
		sourceIp, err := fetchSourceIp(entry)
		if err != nil {
			return nil, err
		}
		if sourceIp == "" {
			continue
		}

		// fetch sourceSvcMeta
		sourceSvcs, err := spliceSourceSvc(sourceIp, ipToSvcCache)
		if err != nil {
			return nil, err
		}

		// fetch fence
		fenceNN, err := spliceSourcefence(sourceIp, ipTofenceCache)
		if err != nil {
			fenceNN = nil
			return nil, err
		}

		if len(sourceSvcs) == 0 && fenceNN == nil {
			continue
		}

		// fetch destinationSvcMeta
		destinationSvcs := spliceDestinationSvc(entry, sourceSvcs, svcToIpsCache, fenceNN, enableShortDomain)
		if len(destinationSvcs) == 0 {
			continue
		}

		// tmpResult: source -> dest
		for _, sourceSvc := range sourceSvcs {
			if _, ok := tmpResult[sourceSvc]; !ok {
				tmpResult[sourceSvc] = make(map[string]int)
			}
			for _, dest := range destinationSvcs {
				tmpResult[sourceSvc][dest] += 1
			}
		}

		// record the source to dest in fenceNN
		if fenceNN != nil {
			nn := fenceNN.String()
			if _, ok := tmpResult[nn]; !ok {
				tmpResult[nn] = make(map[string]int)
			}
			for _, dest := range destinationSvcs {
				tmpResult[nn][dest] += 1
			}
		}
	}

	for sourceSvc, dstSvcMappings := range tmpResult {
		result[sourceSvc] = make(map[string]string)
		for dstSvc, count := range dstSvcMappings {
			result[sourceSvc][dstSvc] = strconv.Itoa(count)
		}
	}

	return result, nil
}

func fetchSourceIp(entry *data_accesslog.HTTPAccessLogEntry) (string, error) {
	log := log.WithField("reporter", "accesslog convertor").WithField("function", "fetchSourceIp")
	if entry.CommonProperties.DownstreamRemoteAddress == nil {
		log.Debugf("DownstreamRemoteAddress is nil, skip")
		return "", nil
	}
	downstreamSock, ok := entry.CommonProperties.DownstreamRemoteAddress.Address.(*envoy_config_core.Address_SocketAddress)
	if !ok {
		return "", stderrors.New("wrong type of DownstreamRemoteAddress")
	}
	if downstreamSock == nil || downstreamSock.SocketAddress == nil {
		return "", stderrors.New("downstream socket address is nil")
	}
	log.Debugf("SourceEp is: %s", downstreamSock.SocketAddress.Address)
	return downstreamSock.SocketAddress.Address, nil
}

func spliceSourceSvc(sourceIp string, ipToSvcCache *IpToSvcCache) ([]string, error) {
	ipToSvcCache.RLock()
	defer ipToSvcCache.RUnlock()

	if svc, ok := ipToSvcCache.Data[sourceIp]; ok {
		keys := make([]string, 0, len(svc))
		for key := range svc {
			keys = append(keys, key)
		}
		return keys, nil
	}

	log.Debugf("svc not found base on sourceIp %s", sourceIp)
	return []string{}, nil
}

func spliceSourcefence(sourceIp string, ipTofence *IpTofence) (*types.NamespacedName, error) {
	ipTofence.RLock()
	defer ipTofence.RUnlock()

	if nn, ok := ipTofence.Data[sourceIp]; ok {
		return &nn, nil
	}
	log.Debugf("fence not found base on sourceIp %s", sourceIp)
	return nil, nil
}

func spliceDestinationSvc(entry *data_accesslog.HTTPAccessLogEntry, sourceSvcs []string, svcToIpsCache *SvcToIpsCache, fenceNN *types.NamespacedName, enableShortDomain bool) []string {
	log = log.WithField("reporter", "accesslog convertor").WithField("function", "spliceDestinationSvc")
	var destSvcs []string

	upstreamCluster := entry.CommonProperties.UpstreamCluster
	parts := strings.Split(upstreamCluster, "|")
	if len(parts) != 4 {
		log.Errorf("UpstreamCluster is wrong: parts number is not 4, skip")
		return destSvcs
	}
	// only handle inbound access log
	if parts[0] != "inbound" {
		log.Debugf("This log is not inbound, skip")
		return destSvcs
	}
	// get destination service info from request.authority
	auth := entry.Request.Authority
	dest := strings.Split(auth, ":")[0]

	// dest is ip address, skip
	if net.ParseIP(dest) != nil {
		log.Debugf("Accesslog is %+v -> %s, in which the destination is not domain, skip", sourceSvcs, dest)
		return destSvcs
	}

	// both short name and k8s fqdn will be added as following

	// expand domain as step2
	var destSvc string
	destParts := strings.Split(dest, ".")

	switch len(destParts) {
	case 1:
		destSvc = completeDestSvcWithDestName(dest, sourceSvcs, fenceNN)
	case 2:
		destSvc = completeDestSvcName(destParts, dest, "svc.cluster.local", svcToIpsCache)
	case 3:
		if destParts[2] == "svc" {
			destSvc = completeDestSvcName(destParts, dest, "cluster.local", svcToIpsCache)
		} else {
			destSvc = dest
		}
	default:
		destSvc = dest
	}

	destSvcs = append(destSvcs, destSvc)
	// We don't know if it is a short domain，
	// So we add the original dest to slice if it is not same with the parsed
	if enableShortDomain {
		if dest != destSvc {
			destSvcs = append(destSvcs, dest)
		}
	}

	result := make([]string, 0)
	for _, svc := range destSvcs {
		result = append(result, fmt.Sprintf("{destination_service=\"%s\"}", svc))
	}
	log.Debugf("DestinationSvc is: %+v", result)
	return result
}

func completeDestSvcName(destParts []string, dest, suffix string, svcToIpsCache *SvcToIpsCache) (destSvc string) {
	svcToIpsCache.RLock()
	defer svcToIpsCache.RUnlock()

	svc := destParts[1] + "/" + destParts[0]
	if _, ok := svcToIpsCache.Data[svc]; ok {
		// dest is abbreviation of service, add suffix
		destSvc = dest + "." + suffix
	} else {
		// not abbreviation of service, no suffix
		destSvc = dest
	}
	return
}

// exact dest ns from sourceSvc and fence，otherwise return the original value
func completeDestSvcWithDestName(dest string, sourceSvcs []string, fenceNN *types.NamespacedName) (destSvc string) {
	destSvc = dest
	if len(sourceSvcs) > 0 {
		srcParts := strings.Split(sourceSvcs[0], "/")
		if len(srcParts) == 2 {
			destSvc = dest + "." + srcParts[0] + ".svc.cluster.local"
		}
	} else if fenceNN != nil {
		destSvc = dest + "." + fenceNN.Namespace + ".svc.cluster.local"
	}
	return
}
