package cache

import (
	"sync"

	cmap "github.com/orcaman/concurrent-map/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/modules/meshregistry/pkg/multicluster"
)

var K8sNodeCaches = &nodeCacheHandler{}

func newNodeCache() *nodeCache {
	nc := &nodeCache{
		cache: cmap.New[*metav1.ObjectMeta](),
	}
	nc.Handler = cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			nc.add(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			nc.update(oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			nc.delete(obj)
		},
	}
	return nc
}

type nodeCache struct {
	cache   cmap.ConcurrentMap[string, *metav1.ObjectMeta]
	Handler cache.ResourceEventHandlerFuncs
}

func (nc *nodeCache) GetAll() cmap.ConcurrentMap[string, *metav1.ObjectMeta] {
	return nc.cache
}

func (nc *nodeCache) Get(ip string) (meta *metav1.ObjectMeta, exist bool) {
	value, exist := nc.cache.Get(ip)
	if exist {
		return value, exist
	}
	return nil, false
}

func (nc *nodeCache) GetHostKey(key string) (string, bool) {
	_, exist := nc.cache.Get(key)
	return key, exist
}

func (nc *nodeCache) add(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if ok {
		nc.cache.Set(node.Name, &node.ObjectMeta)
	}
}

func (nc *nodeCache) update(oldObj, newObj interface{}) {
	node, ok := newObj.(*v1.Node)
	if ok {
		nc.cache.Set(node.Name, &node.ObjectMeta)
	}
}

func (nc *nodeCache) delete(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if ok {
		nc.cache.Remove(node.Name)
	}
}

type nodeCacheHandler struct {
	sync.Mutex
	caches caches[*metav1.ObjectMeta]
}

func (nch *nodeCacheHandler) GetAll() map[string]cmap.ConcurrentMap[string, *metav1.ObjectMeta] {
	nch.Lock()
	defer nch.Unlock()
	return nch.caches.Get()
}

// Note: Use IP as cache key in single cluster, this interface does not work in multi-cluster multi-network environments
func (nch *nodeCacheHandler) Get(ip string) (meta *metav1.ObjectMeta, exist bool) {
	nch.Lock()
	defer nch.Unlock()
	for _, nodes := range nch.caches {
		meta, exist := nodes.Get(ip)
		if exist {
			return meta, exist
		}
	}
	return nil, false
}

func (nch *nodeCacheHandler) GetHostKey(ip string) (string, bool) {
	nch.Lock()
	defer nch.Unlock()
	for _, nodes := range nch.caches {
		host, exist := nodes.GetHostKey(ip)
		if exist {
			return host, exist
		}
	}
	return "", false
}

func (nch *nodeCacheHandler) ClusterAdded(cluster *multicluster.Cluster, stopCh <-chan struct{}) error {
	nodeInformer := cluster.KubeInformer.Core().V1().Nodes().Informer()
	nch.Lock()
	defer nch.Unlock()
	if nch.caches == nil {
		nch.caches = make(map[string]objectCache[*metav1.ObjectMeta])
	}
	nodeCache := newNodeCache()
	nch.caches[cluster.ID] = nodeCache
	nodeInformer.AddEventHandler(nodeCache.Handler)
	go nodeInformer.Run(stopCh)
	return nil
}

func (nch *nodeCacheHandler) ClusterUpdated(cluster *multicluster.Cluster, clusterStopCh <-chan struct{}) error {
	if err := nch.ClusterDeleted(cluster.ID); err != nil {
		return err
	}
	return nch.ClusterAdded(cluster, clusterStopCh)
}

func (nch *nodeCacheHandler) ClusterDeleted(clusterID string) error {
	nch.Lock()
	defer nch.Unlock()
	delete(nch.caches, clusterID)
	return nil
}
