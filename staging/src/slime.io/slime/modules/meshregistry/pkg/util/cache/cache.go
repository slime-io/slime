package cache

import (
	cmap "github.com/orcaman/concurrent-map"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type caches map[string]objectCache

func (cs caches) Get() map[string]interface{} {
	ret := make(map[string]interface{}, len(cs))
	for k, v := range cs {
		ret[k] = v.GetAll()
	}
	return ret
}

type objectCache interface {
	GetAll() cmap.ConcurrentMap
	Get(string) (*metav1.ObjectMeta, bool)
	GetHostKey(string) (string, bool)
}
