// nolint: unused
package cache

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/modules/meshregistry/pkg/multicluster"
)

var K8sNodeCaches = &cacheHandler[*metav1.ObjectMeta, *v1.Node]{
	objectHandler: nodeHandler{},
}

type nodeHandler struct{}

func (h nodeHandler) informer(cluster *multicluster.Cluster) cache.SharedIndexInformer {
	return cluster.KubeInformer.Core().V1().Nodes().Informer()
}

func (h nodeHandler) meta(meta *metav1.ObjectMeta) *metav1.ObjectMeta {
	return meta
}

func (h nodeHandler) hostKey(meta *metav1.ObjectMeta) string {
	return meta.Name
}

func (h nodeHandler) onAdd(n *v1.Node) (string, *metav1.ObjectMeta, bool) {
	return h.onUpdate(nil, n)
}

func (h nodeHandler) onUpdate(_, n *v1.Node) (string, *metav1.ObjectMeta, bool) {
	if n != nil {
		n.ObjectMeta.ManagedFields = nil // ignore managed fields
		return n.Name, &n.ObjectMeta, true
	}
	return "", nil, false
}

func (h nodeHandler) onDelete(n *v1.Node) (string, bool) {
	if n != nil {
		return n.Name, true
	}
	return "", false
}
