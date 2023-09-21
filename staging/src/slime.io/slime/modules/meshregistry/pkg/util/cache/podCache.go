package cache

import (
	"sync"

	cmap "github.com/orcaman/concurrent-map"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/resource"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/modules/meshregistry/pkg/multicluster"
)

var K8sPodCaches = &podCacheHandler{}

func newPodCache() *podCache {
	pc := &podCache{
		cache: cmap.New(),
	}
	pc.Handler = cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pc.add(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pc.update(oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			pc.delete(obj)
		},
	}
	return pc
}

type podWrapper struct {
	Meta     *metav1.ObjectMeta
	NodeName string
}

type podCache struct {
	cache   cmap.ConcurrentMap
	Handler cache.ResourceEventHandlerFuncs
}

func (pc *podCache) GetAll() cmap.ConcurrentMap {
	return pc.cache
}

func (pc *podCache) Get(ip string) (meta *metav1.ObjectMeta, exist bool) {
	value, exist := pc.cache.Get(ip)
	if exist {
		v, castok := value.(*podWrapper)
		if castok {
			return v.Meta, exist
		}
	}
	return nil, false
}

func (pc *podCache) GetHostKey(ip string) (string, bool) {
	value, exist := pc.cache.Get(ip)
	if exist {
		v, castok := value.(*podWrapper)
		if castok {
			return v.NodeName, true
		}
	}
	return "", false
}

func (pc *podCache) add(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if ok {
		ip := pod.Status.PodIP
		if ip == "" {
			// log.Warnf("pod %s/%s has no ip when add", pod.Namespace, pod.Name)
			return
		}
		if pod.Status.Phase != v1.PodRunning {
			// log.Warnf("pod %s/%s is not running when add", pod.Namespace, pod.Name)
			return
		}
		pc.cache.Set(ip, &podWrapper{&pod.ObjectMeta, pod.Spec.NodeName})
	}
}

func (pc *podCache) update(oldObj, newObj interface{}) {
	pod, ok := newObj.(*v1.Pod)
	if ok {
		ip := pod.Status.PodIP
		if ip == "" {
			// log.Warnf("pod %s/%s has no ip when update", pod.Namespace, pod.Name)
			return
		}
		if pod.Status.Phase != v1.PodRunning {
			// log.Warnf("pod %s/%s is not running when update", pod.Namespace, pod.Name)
			return
		}
		pc.cache.Set(ip, &podWrapper{&pod.ObjectMeta, pod.Spec.NodeName})
	}
}

func (pc *podCache) delete(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if ok {
		ip := pod.Status.PodIP
		if ip == "" {
			// log.Warnf("pod %s/%s has no ip when delete", pod.Namespace, pod.Name)
			return
		}
		pc.cache.Remove(ip)
	}
}

type podCacheHandler struct {
	sync.Mutex
	caches map[string]objectCache
}

func (pch *podCacheHandler) GetAll() map[string]interface{} {
	pch.Lock()
	defer pch.Unlock()
	return (caches)(pch.caches).Get()
}

// Note: Use IP as cache key in flat networks, this interface does not work in multi-cluster multi-network environments
func (pch *podCacheHandler) Get(ip string) (meta *metav1.ObjectMeta, exist bool) {
	pch.Lock()
	defer pch.Unlock()
	for _, pods := range pch.caches {
		meta, exist := pods.Get(ip)
		if exist {
			return meta, exist
		}
	}
	return nil, false
}

// Note: Use IP as cache key in flat networks, this interface does not work in multi-cluster multi-network environments
func (pch *podCacheHandler) GetHostKey(ip string) (string, bool) {
	pch.Lock()
	defer pch.Unlock()
	for _, pods := range pch.caches {
		host, exist := pods.GetHostKey(ip)
		if exist {
			return host, exist
		}
	}
	return "", false
}

func (pch *podCacheHandler) ClusterAdded(cluster *multicluster.Cluster, stopCh <-chan struct{}) error {
	podKubeResource := collection.Builder{
		Name: "k8s/core/v1/pods",
		Resource: resource.Builder{
			Group:   "",
			Version: "",
			Plural:  "Pods",
			Kind:    "Pod",
		}.BuildNoValidate(),
	}.MustBuild()
	podInformer, err := cluster.Provider.GetAdapter(podKubeResource.Resource()).NewInformer()
	if err != nil {
		log.Errorf("cluster %s create k8s pod informer error: %v", cluster.ID, err)
		return err
	}

	pch.Lock()
	defer pch.Unlock()
	if pch.caches == nil {
		pch.caches = make(map[string]objectCache)
	}
	podCache := newPodCache()
	pch.caches[cluster.ID] = podCache
	podInformer.AddEventHandler(podCache.Handler)
	go podInformer.Run(stopCh)
	return nil
}

func (pch *podCacheHandler) ClusterUpdated(cluster *multicluster.Cluster, clusterStopCh <-chan struct{}) error {
	if err := pch.ClusterDeleted(cluster.ID); err != nil {
		return err
	}
	return pch.ClusterAdded(cluster, clusterStopCh)
}

func (pch *podCacheHandler) ClusterDeleted(clusterID string) error {
	pch.Lock()
	defer pch.Unlock()
	delete(pch.caches, clusterID)
	return nil
}
