package k8s

import (
	"sync"
	"time"

	"github.com/orcaman/concurrent-map"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"yun.netease.com/slime/pkg/controller/destinationrule"
	"yun.netease.com/slime/pkg/model/source"
	"yun.netease.com/slime/pkg/util"
)

type Source struct {
	EventChan        chan<- source.Event
	K8sClient        []*kubernetes.Clientset
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
	destinationrule.HostSubsetMapping.Subscribe(m.subscribe)
	go func() {
		for {
			select {
			case <-stop:
				return
			case e := <-m.Watcher.ResultChan():
				m.watchHandler(m, e)
			case <-ticker.C:
				m.timerHandler(m)
			case loc := <-m.UpdateChan:
				m.updateHandler(m, loc)
			}
		}
	}()
}

// 将svc资源加入到监控关心的列表中
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
