package k8s

import (
	"context"
	cmap "github.com/orcaman/concurrent-map"
	prometheusApi "github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	prometheusModel "github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
	"istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"reflect"
	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/framework/util"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MetricSource struct {
	name                     string
	gvks                     []schema.GroupVersionKind
	metricUpdateCheckHandler func(event model.WatcherEvent) map[string]*bootconfig.Prometheus_Source_Handler
	moduleEventChan          chan<- model.ModuleEvent
	restConfig               rest.Config
	env                      *bootstrap.Environment
	api                      prometheus.API
	interest                 cmap.ConcurrentMap
	sync.RWMutex
}

// Register Module -> MetricSource
func Register(mo module.Module) (*MetricSource, error) {
	log := log.WithField("Module Register", mo.Name())
	m := &MetricSource{}
	m.name = mo.Name()
	m.gvks = mo.GVKs()
	m.metricUpdateCheckHandler = mo.MetricUpdateCheckHandler()
	m.moduleEventChan = mo.ModuleEventChan()
	m.restConfig = mo.RestConfig()
	m.env = mo.Env()

	// init m.api from env
	if ps := mo.Env().Config.Metric.Prometheus; ps != nil {
		promClient, err := prometheusApi.NewClient(prometheusApi.Config{
			Address:      ps.Address,
			RoundTripper: nil,
		})
		if err != nil {
			log.Errorf("failed to create prometheus client, %+v", err)
		} else {
			m.api = prometheus.NewAPI(promClient)
			log.Infof("successfully create prometheus client")
		}
	}

	// init m.interest
	m.interest = cmap.New()

	return m, nil
}

func (m *MetricSource) getInterest() cmap.ConcurrentMap {
	return m.interest
}

// InterestAdd add resource to interest
func (m *MetricSource) InterestAdd(nn types.NamespacedName) {
	m.interest.Set(nn.Namespace+"/"+nn.Name, true)
}

// InterestRemove remove from interest, the resource will be deleted by k8s
func (m *MetricSource) InterestRemove(nn types.NamespacedName) {
	m.interest.Pop(nn.Namespace + "/" + nn.Name)
}

func (m *MetricSource) interestGet(nn types.NamespacedName) bool {
	_, result := m.interest.Get(nn.Namespace + "/" + nn.Name)
	return result
}

func (m *MetricSource) Name() string {
	return m.name
}

func (m *MetricSource) RestConfig() rest.Config {
	return m.restConfig
}

func (m *MetricSource) GVKs() []schema.GroupVersionKind {
	return m.gvks
}

// Notify push ModuleEvent to module
func (m *MetricSource) Notify(we model.WatcherEvent) {
	// event preCheck
	if m.gvkCheck(we.GVK) && m.interestGet(we.NN) {
		// handler real watcher event
		log.Debugf("handler real watcher event\n")
		mapS2H := m.metricUpdateCheckHandler(we)
		if mapS2H == nil {
			return
		}
		material := m.getMetrics(mapS2H, we.NN)
		log.Debugf("send ModuleEvent for %s", we.NN.String())
		m.moduleEventChan <- model.ModuleEvent{
			NN:       we.NN,
			Material: material,
		}
		return
	}
	// handler fake watcher event - timer event
	if reflect.DeepEqual(we, model.WatcherEvent{}) {
		log.Debugf("handler fake watcher event\n")
		mapS2H := m.metricUpdateCheckHandler(we)
		if mapS2H == nil {
			// module refuse to handler fake watcher event
			return
		}
		m.RLock()
		for k := range m.getInterest().Items() {
			if index := strings.Index(k, "/"); index == -1 || index == len(k)-1 {
				continue
			} else {
				ns := k[:index]
				name := k[index+1:]
				nn := types.NamespacedName{Namespace: ns, Name: name}
				material := m.getMetrics(mapS2H, nn)
				log.Debugf("send ModuleEvent for %s", nn.String())
				m.moduleEventChan <- model.ModuleEvent{
					NN:       nn,
					Material: material,
				}
			}
		}
		m.RUnlock()
		return
	}
	log.Debugf("no interest in this watcher event content\n")
}

func (m *MetricSource) gvkCheck(gvk schema.GroupVersionKind) bool {
	for _, g := range m.gvks {
		if g.Group == gvk.Group && g.Version == gvk.Version && g.Kind == gvk.Kind {
			return true
		}
	}
	return false
}

// 获取新指标
func (m *MetricSource) getMetrics(psh map[string]*bootconfig.Prometheus_Source_Handler, nn types.NamespacedName) map[string]string {
	// todo metric: GetHandler change
	log := log.WithField("getMetrics", nn)
	material := make(map[string]string)

	if _, ok := m.interest.Get(nn.Namespace + "/" + nn.Name); !ok {
		return material
	}
	pods := make([]v1.Pod, 0)
	var service *v1.Service
	c := m.env.K8SClient
	ps, err := c.CoreV1().Pods(nn.Namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Errorf("query pod list faild, %+v", err)
		return map[string]string{}
	}
	pods = append(pods, ps.Items...)

	s, err := c.CoreV1().Services(nn.Namespace).Get(nn.Name, metav1.GetOptions{})
	if err == nil {
		service = s
	}

	if service != nil {
		subsetsPods := make(map[string][]string)
		for _, pod := range pods {
			if util.IsContain(pod.Labels, service.Spec.Selector) && pod.DeletionTimestamp == nil {
				host := util.UnityHost(nn.Name, nn.Namespace)
				if controllers.HostSubsetMapping.Get(host) != nil {
					if sbs, ok := controllers.HostSubsetMapping.Get(host).([]*v1alpha3.Subset); ok {
						for _, sb := range sbs {
							if util.IsContain(pod.Labels, sb.Labels) {
								if subsetsPods[sb.Name] != nil {
									subsetsPods[sb.Name] = append(subsetsPods[sb.Name], pod.Name)
								} else {
									subsetsPods[sb.Name] = []string{pod.Name}
								}
							}
						}
					}
				}
				if subsetsPods[util.Wellkonw_BaseSet] != nil {
					subsetsPods[util.Wellkonw_BaseSet] = append(subsetsPods[util.Wellkonw_BaseSet], pod.Name)
				} else {
					subsetsPods[util.Wellkonw_BaseSet] = []string{pod.Name}
				}
			}
		}
		m.processSubsetPod(psh, subsetsPods, service, material)
	} else {
		log.Error("Service Not Found")
	}

	return material
}

func (m *MetricSource) processSubsetPod(psh map[string]*bootconfig.Prometheus_Source_Handler, subsetsPods map[string][]string, svc *v1.Service, material map[string]string) {
	if material == nil {
		return
	}
	for k, v := range psh {
		// This is inline handler
		if k == "pod" {
			for subsetName, subsetPods := range subsetsPods {
				material[subsetName+".pod"] = strconv.Itoa(len(subsetPods))
			}
			continue
		}
		if v.Query == "" {
			continue
		}
		query := strings.ReplaceAll(v.Query, "$namespace", svc.Namespace)
		// TODO: Use more accurate replacements
		query = strings.ReplaceAll(query, "$source_app", svc.Name)
		switch v.Type {
		case bootconfig.Prometheus_Source_Value:
			if k == "" {
				log.Error("invalid query,value type must have a item")
			}
			// Could be grouped by subset
			if strings.Contains(v.Query, "$pod_name") {
				for subsetName, subsetPods := range subsetsPods {
					subQuery := strings.ReplaceAll(query, "$pod_name", strings.Join(subsetPods, "|"))
					if result := m.queryValue(subQuery); result != "" {
						material[subsetName+"."+k] = result
					}
				}
			} else {
				if result := m.queryValue(query); result != "" {
					material[k] = result
				}
			}
		case bootconfig.Prometheus_Source_Group:
			for k, v := range m.queryGroup(query) {
				material[k] = v
			}
		}
	}
}

func (m *MetricSource) queryValue(q string) string {
	qv, w, e := m.api.Query(context.Background(), q, time.Now())
	if e != nil {
		log.Errorf("failed get metric from prometheus, %+v", e)
		return ""
	} else if w != nil {
		log.Errorf("%s, failed get metric from prometheus", strings.Join(w, ";"))
		return ""
	} else {
		switch qv.Type() {
		case prometheusModel.ValVector:
			vector := qv.(prometheusModel.Vector)
			if vector.Len() == 0 {
				log.Infof("query: %s, No data", q)
				return ""
			}
			if vector.Len() != 1 {
				log.Errorf("invalid query, query: %s, You need to sum up the monitoring data", q)
				return ""
			}
			return vector[0].Value.String()
		}
	}
	return ""
}

func (m *MetricSource) queryGroup(q string) map[string]string {
	qv, w, e := m.api.Query(context.Background(), q, time.Now())
	result := make(map[string]string)
	if e != nil {
		log.Errorf("failed get metric from prometheus, %+v", e)
		return nil
	} else if w != nil {
		log.Errorf("%s,failed get metric from prometheus", strings.Join(w, ";"))
		return nil
	} else {
		switch qv.Type() {
		case prometheusModel.ValVector:
			vector := qv.(prometheusModel.Vector)
			for _, vx := range vector {
				result[vx.Metric.String()] = vx.Value.String()
			}
		}
	}
	return result
}
