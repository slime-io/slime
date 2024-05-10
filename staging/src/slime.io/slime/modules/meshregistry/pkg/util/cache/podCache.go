// nolint: unused
package cache

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/modules/meshregistry/pkg/multicluster"
)

var K8sPodCaches = &cacheHandler[*podWrapper, *v1.Pod]{
	objectHandler: podCacheHandler{},
}

type podWrapper struct {
	Meta     *metav1.ObjectMeta
	NodeName string
}

type podCacheHandler struct{}

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
	ip := p.Status.PodIP
	if ip == "" {
		return "", nil, false
	}
	if p.Status.Phase != v1.PodRunning {
		return "", nil, false
	}
	p.ObjectMeta.ManagedFields = nil // ignore managed fields
	return ip, &podWrapper{&p.ObjectMeta, p.Spec.NodeName}, true
}

func (h podCacheHandler) onDelete(p *v1.Pod) (string, bool) {
	if p == nil {
		return "", false
	}
	ip := p.Status.PodIP
	if ip == "" {
		return "", false
	}
	return ip, true
}
