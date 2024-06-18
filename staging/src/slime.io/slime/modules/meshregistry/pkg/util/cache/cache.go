package cache

import (
	"sync"

	cmap "github.com/orcaman/concurrent-map/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/multicluster"
)

var log = model.ModuleLog.WithField("util", "cache") //nolint: unused

type objecjHandler[V, O any] interface {
	informer(cluster *multicluster.Cluster) cache.SharedIndexInformer
	meta(V) *metav1.ObjectMeta
	hostKey(V) string
	onAdd(O) (string, V, bool)
	onUpdate(O, O) (string, V, bool)
	onDelete(O) (string, bool)
}

type objectCache[V, O any] struct {
	cache      cmap.ConcurrentMap[string, V]
	hasSynced  func() bool
	handler    cache.ResourceEventHandlerFuncs
	objHandler objecjHandler[V, O]
}

func newObjectCache[V, O any](objHandler objecjHandler[V, O]) *objectCache[V, O] {
	ret := &objectCache[V, O]{
		objHandler: objHandler,
		cache:      cmap.New[V](),
	}
	ret.handler = cache.ResourceEventHandlerFuncs{
		AddFunc:    ret.add,
		UpdateFunc: ret.update,
		DeleteFunc: ret.delete,
	}
	return ret
}

func (oc *objectCache[V, O]) GetAll() cmap.ConcurrentMap[string, V] {
	return oc.cache
}

func (oc *objectCache[V, O]) Get(key string) (*metav1.ObjectMeta, bool) {
	v, exist := oc.cache.Get(key)
	if !exist {
		return nil, false
	}
	return oc.objHandler.meta(v), true
}

func (oc *objectCache[V, O]) GetHostKey(key string) (string, bool) {
	v, exist := oc.cache.Get(key)
	if !exist {
		return "", false
	}
	return oc.objHandler.hostKey(v), true
}

func (oc *objectCache[V, O]) HasSynced() bool {
	return oc.hasSynced()
}

func (oc *objectCache[V, O]) add(obj interface{}) {
	t, ok := obj.(O)
	if !ok {
		return
	}
	k, v, ok := oc.objHandler.onAdd(t)
	if !ok {
		return
	}
	oc.cache.Set(k, v)
}

func (oc *objectCache[V, O]) update(oldObj, newObj interface{}) {
	oldT, ok := oldObj.(O)
	if !ok {
		return
	}
	newT, ok := newObj.(O)
	if !ok {
		return
	}
	k, v, ok := oc.objHandler.onUpdate(oldT, newT)
	if !ok {
		return
	}
	oc.cache.Set(k, v)
}

func (oc *objectCache[V, O]) delete(obj interface{}) {
	t, ok := obj.(O)
	if !ok {
		return
	}
	k, ok := oc.objHandler.onDelete(t)
	if !ok {
		return
	}
	oc.cache.Remove(k)
}

type cacheHandler[V, O any] struct {
	sync.Mutex
	caches        map[string]*objectCache[V, O]
	objectHandler objecjHandler[V, O]
}

func (ch *cacheHandler[V, O]) GetAll() map[string]cmap.ConcurrentMap[string, V] {
	ch.Lock()
	defer ch.Unlock()
	ret := make(map[string]cmap.ConcurrentMap[string, V], len(ch.caches))
	for k, v := range ch.caches {
		ret[k] = v.GetAll()
	}
	return ret
}

func (ch *cacheHandler[V, O]) Get(key string) (*metav1.ObjectMeta, bool) {
	ch.Lock()
	defer ch.Unlock()
	for _, c := range ch.caches {
		meta, exist := c.Get(key)
		if exist {
			return meta, exist
		}
	}
	return nil, false
}

func (ch *cacheHandler[V, O]) GetHostKey(key string) (string, bool) {
	ch.Lock()
	defer ch.Unlock()
	for _, c := range ch.caches {
		host, exist := c.GetHostKey(key)
		if exist {
			return host, exist
		}
	}
	return "", false
}

func (ch *cacheHandler[V, O]) ClusterAdded(cluster *multicluster.Cluster, stop <-chan struct{}) error {
	informer := ch.objectHandler.informer(cluster)
	ch.Lock()
	defer ch.Unlock()
	if ch.caches == nil {
		ch.caches = make(map[string]*objectCache[V, O])
	}
	ncache := newObjectCache[V, O](ch.objectHandler)
	ncache.hasSynced = informer.HasSynced
	ch.caches[cluster.ID] = ncache
	informer.AddEventHandler(ncache.handler)
	go informer.Run(stop)
	return nil
}

func (ch *cacheHandler[V, O]) ClusterUpdated(cluster *multicluster.Cluster, clusterStopCh <-chan struct{}) error {
	if err := ch.ClusterDeleted(cluster.ID); err != nil {
		return err
	}
	return ch.ClusterAdded(cluster, clusterStopCh)
}

func (ch *cacheHandler[V, O]) ClusterDeleted(clusterID string) error {
	ch.Lock()
	defer ch.Unlock()
	delete(ch.caches, clusterID)
	return nil
}

func (ch *cacheHandler[V, O]) HasSynced() bool {
	ch.Lock()
	defer ch.Unlock()
	for _, c := range ch.caches {
		if !c.HasSynced() {
			return false
		}
	}
	return true
}
