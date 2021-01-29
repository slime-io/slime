package metric

import (
	"encoding/json"
	cmap "github.com/orcaman/concurrent-map"
	"istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"strings"
	"time"
	"yun.netease.com/slime/pkg/controller/destinationrule"
	"yun.netease.com/slime/pkg/model/source"
	"yun.netease.com/slime/pkg/model/source/k8s"
	"yun.netease.com/slime/pkg/util"

	"k8s.io/client-go/kubernetes"
)

var log = logf.Log.WithName("source_k8s_metric_source")

// PodMetricsList : PodMetricsList
type PodMetricsList struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Metadata   struct {
		SelfLink string `json:"selfLink"`
	} `json:"metadata"`
	Items []struct {
		Metadata struct {
			Name              string    `json:"name"`
			Namespace         string    `json:"namespace"`
			SelfLink          string    `json:"selfLink"`
			CreationTimestamp time.Time `json:"creationTimestamp"`
		} `json:"metadata"`
		Timestamp  time.Time `json:"timestamp"`
		Window     string    `json:"window"`
		Containers []struct {
			Name  string `json:"name"`
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"containers"`
	} `json:"items"`
}

type PodMetrics struct {
	Metadata struct {
		Name              string    `json:"name"`
		Namespace         string    `json:"namespace"`
		SelfLink          string    `json:"selfLink"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
	} `json:"metadata"`
	Timestamp  time.Time `json:"timestamp"`
	Window     string    `json:"window"`
	Containers []struct {
		Name  string `json:"name"`
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"containers"`
}

func getMetrics(clientset *kubernetes.Clientset, pods *PodMetrics, namespace, podname string) error {
	data, err := clientset.RESTClient().Get().AbsPath("apis/metrics.k8s.io/v1beta1/namespaces/" + namespace + "/pods/" + podname).DoRaw()
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &pods)
	return err
}

func cpuToInt(CPU string) (int, error) {
	if strings.HasSuffix(CPU, "n") {
		CPU = CPU[:len(CPU)-2]
		return strconv.Atoi(CPU)
	}
	return strconv.Atoi(CPU)
}

func memoryToInt(memory string) (int, error) {
	if strings.HasSuffix(memory, "Ki") {
		memory = memory[:len(memory)-3]
		return strconv.Atoi(memory)
	}
	return strconv.Atoi(memory)
}

func NewMetricSource(c *kubernetes.Clientset, eventChan chan source.Event) (*k8s.Source, error) {
	watcher, err := c.CoreV1().Endpoints("").Watch(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	es := &k8s.Source{
		EventChan:  eventChan,
		Watcher:    watcher,
		K8sClient:  []*kubernetes.Clientset{c},
		UpdateChan: make(chan types.NamespacedName),
		Interest:   cmap.New(),
	}
	es.SetHandler(metricGetHandler, metricWatcherHandler, metricTimerHandler, update)
	return es, nil
}

// metric source handlers
func metricWatcherHandler(m *k8s.Source, e watch.Event) {
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

func metricTimerHandler(m *k8s.Source) {
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

func metricGetHandler(m *k8s.Source, meta types.NamespacedName) map[string]string {
	reqLogger := log.WithValues("Request.Namespace", meta.Namespace, "Request.Name", meta.Name)
	material := make(map[string]string)
	if _, ok := m.Interest.Get(meta.Namespace + "/" + meta.Name); !ok {
		return material
	}
	CPUMax := 0
	CPUSum := 0
	MemoryMax := 0
	MemorySum := 0
	PodNum := 0
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
		for _, pod := range pods {
			if util.IsContain(pod.Labels, service.Spec.Selector) && pod.DeletionTimestamp == nil {
				host := util.UnityHost(meta.Name, meta.Namespace)
				if destinationrule.HostSubsetMapping.Get(host) != nil {
					if sbs, ok := destinationrule.HostSubsetMapping.Get(host).([]*v1alpha3.Subset); ok {
						for _, sb := range sbs {
							subPod := 0
							subPod++
							material[sb.Name+".pod"] = strconv.Itoa(subPod)
						}
					}
				}

				var metrics PodMetrics
				var err error
				for _, k := range m.K8sClient {
					err = getMetrics(k, &metrics, pod.Namespace, pod.Name)
					if err == nil {
						for _, v := range metrics.Containers {
							c, _ := cpuToInt(v.Usage.CPU)
							m, _ := memoryToInt(v.Usage.Memory)

							if c > CPUMax {
								CPUMax = c
							}
							CPUSum = CPUSum + c

							if m > MemoryMax {
								MemoryMax = m
							}
							MemorySum = MemorySum + m
						}
					}
				}
				PodNum = PodNum + 1
			}
		}
		material["cpu.sum"] = strconv.Itoa(CPUSum)
		material["memory.sum"] = strconv.Itoa(MemorySum)
		material["cpu.max"] = strconv.Itoa(CPUMax)
		material["memory.max"] = strconv.Itoa(MemoryMax)
		material["pod"] = strconv.Itoa(PodNum)
	} else {
		reqLogger.Error(nil, "Service Not Found")
	}
	return material
}

func update(m *k8s.Source, loc types.NamespacedName) {
	material := m.Get(loc)
	if material["pod"] == "0" || material["pod"] == "" {
		return
	}
	m.EventChan <- source.Event{
		EventType: source.Add,
		Loc:       loc,
		Info:      material,
	}
}
