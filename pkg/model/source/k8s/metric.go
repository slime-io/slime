package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"slime.io/slime/pkg/apis/config/v1alpha1"

	"github.com/prometheus/common/model"
	"istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"slime.io/slime/pkg/controller/destinationrule"
	"slime.io/slime/pkg/model/source"
	"slime.io/slime/pkg/util"
)

var log = logf.Log.WithName("source_k8s_metric_source")

// metric source handlers
func metricWatcherHandler(m *Source, e watch.Event) {
	if e.Object == nil {
		return
	}
	ep, ok := e.Object.(*v1.Endpoints)
	if !ok {
		return
	}

	loc := types.NamespacedName{
		Namespace: ep.Namespace,
		Name:      ep.Name,
	}
	if _, exist := m.Interest.Get(loc.Namespace + "/" + loc.Name); !exist {
		return
	}
	update(m, loc)
}

func metricTimerHandler(m *Source) {
	m.RLock()
	for k := range m.Interest.Items() {
		if index := strings.Index(k, "/"); index == -1 || index == len(k)-1 {
			continue
		} else {
			ns := k[:index]
			name := k[index+1:]
			update(m, types.NamespacedName{
				Namespace: ns,
				Name:      name,
			})
		}
	}
	m.RUnlock()
}

func metricGetHandler(m *Source, meta types.NamespacedName) map[string]string {
	reqLogger := log.WithValues("Request.Namespace", meta.Namespace, "Request.Name", meta.Name)
	material := make(map[string]string)
	if _, ok := m.Interest.Get(meta.Namespace + "/" + meta.Name); !ok {
		return material
	}
	pods := make([]v1.Pod, 0)
	var service *v1.Service
	for _, c := range m.K8sClient {
		ps, err := c.CoreV1().Pods(meta.Namespace).List(metav1.ListOptions{})
		if err != nil {
			log.Error(err, "获取pod列表失败")
			continue
		}
		pods = append(pods, ps.Items...)

		s, err := c.CoreV1().Services(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		if err == nil {
			service = s
			break
		}
		// 若当前集群未找到则查找下一个集群
		if !errors.IsNotFound(err) {
			log.Error(err, "获取Service失败："+meta.Name)
		}
	}
	if service != nil {
		subsetsPods := make(map[string][]string)
		for _, pod := range pods {
			if util.IsContain(pod.Labels, service.Spec.Selector) && pod.DeletionTimestamp == nil {
				host := util.UnityHost(meta.Name, meta.Namespace)
				if destinationrule.HostSubsetMapping.Get(host) != nil {
					if sbs, ok := destinationrule.HostSubsetMapping.Get(host).([]*v1alpha3.Subset); ok {
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
		m.processSubsetPod(subsetsPods, service, material)
	} else {
		reqLogger.Error(nil, "Service Not Found")
	}
	return material
}

func (m *Source) processSubsetPod(subsetsPods map[string][]string, svc *v1.Service, material map[string]string) {
	if material == nil {
		return
	}
	for k, v := range m.items {
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
		case v1alpha1.Prometheus_Source_Value:
			if k == "" {
				log.Error(fmt.Errorf("Invaild Query"), "value type must have a item")
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
		case v1alpha1.Prometheus_Source_Group:
			for k, v := range m.queryGroup(query) {
				material[k] = v
			}
		}
	}
}

func (m *Source) queryValue(q string) string {
	qv, w, e := m.api.Query(context.Background(), q, time.Now())
	if e != nil {
		log.Error(e, "failed get metric from prometheus")
		return ""
	} else if w != nil {
		log.Error(fmt.Errorf(strings.Join(w, ";")), "failed get metric from prometheus")
		return ""
	} else {
		switch qv.Type() {
		case model.ValVector:
			vector := qv.(model.Vector)
			if vector.Len() != 1 {
				log.Error(fmt.Errorf("Invaild Query"), "You need to sum up the monitoring data")
			}
			return vector[0].Value.String()
		}
	}
	return ""
}

func (m *Source) queryGroup(q string) map[string]string {
	qv, w, e := m.api.Query(context.Background(), q, time.Now())
	result := make(map[string]string)
	if e != nil {
		log.Error(e, "failed get metric from prometheus")
		return nil
	} else if w != nil {
		log.Error(fmt.Errorf(strings.Join(w, ";")), "failed get metric from prometheus")
		return nil
	} else {
		switch qv.Type() {
		case model.ValVector:
			vector := qv.(model.Vector)
			for _, vx := range vector {
				result[vx.Metric.String()] = vx.Value.String()
			}
		}
	}
	return result
}

func update(m *Source, loc types.NamespacedName) {
	material := m.Get(loc)
	m.EventChan <- source.Event{
		EventType: source.Add,
		Loc:       loc,
		Info:      material,
	}
}
