package k8s

import (
	"context"
	"sync"
	"time"

	"github.com/orcaman/concurrent-map"
	prometheus_client "github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model/source"
	"slime.io/slime/framework/util"
)

type Source struct {
	EventChan chan<- source.Event
	K8sClient []*kubernetes.Clientset
	api       prometheus.API
	//
	items            map[string]*v1alpha1.Prometheus_Source_Handler
	Watcher          watch.Interface
	Interest         cmap.ConcurrentMap
	UpdateChan       chan types.NamespacedName
	multiClusterLock sync.RWMutex
	getHandler       func(*Source, types.NamespacedName) map[string]string
	watchHandler     func(*Source, watch.Event)
	timerHandler     func(*Source)
	updateHandler    func(*Source, types.NamespacedName)
	sync.RWMutex
}

func (m *Source) SetHandler(
	getHandler func(*Source, types.NamespacedName) map[string]string,
	watchHandler func(*Source, watch.Event),
	timerHandler func(*Source),
	updateHandler func(*Source, types.NamespacedName)) {
	m.getHandler = getHandler
	m.watchHandler = watchHandler
	m.timerHandler = timerHandler
	m.updateHandler = updateHandler
}

func (m *Source) Start(stop <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	controllers.HostSubsetMapping.Subscribe(m.subscribe)
	go func() {
		for {
			select {
			case <-stop:
				return
			case e, ok := <-m.Watcher.ResultChan():
				if !ok {
					log.Warningf("Source watcher result chan closed, break process loop")
					return
				}
				m.watchHandler(m, e)
			case <-ticker.C:
				m.timerHandler(m)
			case loc := <-m.UpdateChan:
				m.updateHandler(m, loc)
			}
		}
	}()
}

// WatchAdd add resource to watch list
func (m *Source) WatchAdd(meta types.NamespacedName) {
	m.Interest.Set(meta.Namespace+"/"+meta.Name, true)
	m.UpdateChan <- meta
}

func (m *Source) Get(meta types.NamespacedName) map[string]string {
	return m.getHandler(m, meta)
}

// K8S负责回收资源，该处只须将其从监控关心列表中移除
func (m *Source) WatchRemove(meta types.NamespacedName) {
	m.Interest.Pop(meta.Namespace + "/" + meta.Name)
}

func (m *Source) SourceClusterHandler() func(*kubernetes.Clientset) {
	f := func(c *kubernetes.Clientset) {
		m.multiClusterLock.Lock()
		m.K8sClient = append(m.K8sClient, c)
		m.multiClusterLock.Unlock()
	}
	return f
}

func (m *Source) subscribe(key string, value interface{}) {
	if name, ns, ok := util.IsK8SService(key); ok {
		m.Get(types.NamespacedName{Namespace: ns, Name: name})
	}
}

func NewMetricSource(eventChan chan source.Event, env *bootstrap.Environment) (*Source, error) {
	if env.Config.Metric == nil {
		return nil, nil
	}

	k8sClient := env.K8SClient
	epsClient := k8sClient.CoreV1().Endpoints("")
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return epsClient.List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return epsClient.Watch(options)
		},
	}
	watcher := util.ListWatcher(context.Background(), lw)

	es := &Source{
		EventChan:  eventChan,
		Watcher:    watcher,
		K8sClient:  []*kubernetes.Clientset{k8sClient},
		UpdateChan: make(chan types.NamespacedName),
		Interest:   cmap.New(),
	}
	if m := env.Config.Metric.Prometheus; m != nil {
		promClient, err := prometheus_client.NewClient(prometheus_client.Config{
			Address:      m.Address,
			RoundTripper: nil,
		})
		if err != nil {
			log.Errorf("failed create prometheus client, %+v", err)
		} else {
			es.api = prometheus.NewAPI(promClient)
			es.items = m.Handlers
		}
	}
	if m := env.Config.Metric.K8S; m != nil {
		for _, v := range m.Handlers {
			// TODO: Transformed into a function
			es.items[v] = &v1alpha1.Prometheus_Source_Handler{}
		}
	}
	es.SetHandler(metricGetHandler, metricWatcherHandler, metricTimerHandler, update)
	return es, nil
}
