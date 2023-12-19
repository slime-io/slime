package cache

import (
	cmap "github.com/orcaman/concurrent-map/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "util/cache")

type caches[V any] map[string]objectCache[V]

func (cs caches[V]) Get() map[string]cmap.ConcurrentMap[string, V] {
	ret := make(map[string]cmap.ConcurrentMap[string, V], len(cs))
	for k, v := range cs {
		ret[k] = v.GetAll()
	}
	return ret
}

type objectCache[V any] interface {
	GetAll() cmap.ConcurrentMap[string, V]
	Get(string) (*metav1.ObjectMeta, bool)
	GetHostKey(string) (string, bool)
}
