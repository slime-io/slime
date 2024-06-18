// nolint: unused
package cache

import (
	cmap "github.com/orcaman/concurrent-map/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/modules/meshregistry/pkg/multicluster"
)

var podCacheLog = log.WithField("type", "pod")

var K8sPodCaches = &cacheHandler[*podWrapper, *v1.Pod]{
	objectHandler: podCacheHandler{
		localCache: cmap.New[*metav1.ObjectMeta](),
	},
}

type podWrapper struct {
	Meta     *metav1.ObjectMeta
	NodeName string
}

type podCacheHandler struct {
	// localCache is the cache of pods in local, it is used to filter pods that needn't be cached.
	// For example, when a job pod's status changes to succeeded, its ip is reclaimed and assigned to another pod.
	// The job pod may be deleted while another pod is running with the same ip, and attempts to trigger
	// cache cleanup due to the job pod's deletion should be ignored.
	// Currently, we don't have a good way to determine which pod the ip was assigned to first.
	// For the time being, we consider the post-created pods to have a higher priority.
	localCache cmap.ConcurrentMap[string, *metav1.ObjectMeta]
}

func (h podCacheHandler) informer(cluster *multicluster.Cluster) cache.SharedIndexInformer {
	return cluster.KubeInformer.Core().V1().Pods().Informer()
}

func (h podCacheHandler) meta(pw *podWrapper) *metav1.ObjectMeta {
	return pw.Meta
}

func (h podCacheHandler) hostKey(pw *podWrapper) string {
	return pw.NodeName
}

func (h podCacheHandler) onAdd(p *v1.Pod) (string, *podWrapper, bool) {
	return h.onUpdate(nil, p)
}

func (h podCacheHandler) onUpdate(_, p *v1.Pod) (string, *podWrapper, bool) {
	if p == nil {
		return "", nil, false
	}
	podCacheLog.Debugf("handle add/update pod %s/%s, HostNetwork: %t, Phase: %s, IP: %s, CreatAt: %s",
		p.Namespace, p.Name, p.Spec.HostNetwork, p.Status.Phase, p.Status.PodIP, p.CreationTimestamp)
	ip := p.Status.PodIP
	if ip == "" {
		return "", nil, false
	}

	// not support host network pod
	if p.Spec.HostNetwork {
		return "", nil, false
	}

	// skip pods that are not running
	if p.Status.Phase != v1.PodRunning {
		return "", nil, false
	}

	if meta, exit := h.localCache.Get(ip); exit {
		if p.Name != meta.Name || p.Namespace != meta.Namespace {
			// current pod is older than the one in local cache, skip it
			if p.CreationTimestamp.Before(&meta.CreationTimestamp) {
				podCacheLog.Warnf("try cache pod %s/%s  with ip %s older than the one %s/%s already exist, skip it",
					p.Namespace, p.Name, ip, meta.Namespace, meta.Name)
				return ip, nil, false
			}
			podCacheLog.Warnf("using newer pod %s/%s override already exist pod %s/%s with same ip %s.",
				p.Namespace, p.Name, meta.Namespace, meta.Name, ip)
		}
		// already cached, may be labels changed, also need to update meta
	}

	podCacheLog.Debugf("cache pod %s/%s with ip %s", p.Namespace, p.Name, ip)
	p.ObjectMeta.ManagedFields = nil // ignore managed fields
	h.localCache.Set(ip, &p.ObjectMeta)
	return ip, &podWrapper{&p.ObjectMeta, p.Spec.NodeName}, true
}

func (h podCacheHandler) onDelete(p *v1.Pod) (string, bool) {
	if p == nil {
		return "", false
	}
	podCacheLog.Debugf("handle delte pod %s/%s,  Phase: %s, IP: %s, CreatAt: %s",
		p.Namespace, p.Name, p.Status.Phase, p.Status.PodIP, p.CreationTimestamp)
	ip := p.Status.PodIP
	if ip == "" {
		return "", false
	}
	if meta, exit := h.localCache.Get(ip); exit {
		if (p.Name != meta.Name || p.Namespace != meta.Namespace) &&
			// current pod is older than the one in local cache, skip it
			p.CreationTimestamp.Before(&meta.CreationTimestamp) {
			podCacheLog.Warnf("try delete pod %s/%s with ip %s older than the one %s/%s, skip it",
				p.Namespace, p.Name, ip, meta.Namespace, meta.Name)
			return ip, false
		}
	}
	podCacheLog.Debugf("delete pod %s/%s with ip %s", p.Namespace, p.Name, ip)
	h.localCache.Remove(ip)
	return ip, true
}
