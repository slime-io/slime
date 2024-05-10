package util

import (
	"fmt"
	"strings"
	"sync"

	cmap "github.com/orcaman/concurrent-map/v2"
)

// Map operation
func IsContain(farther, child map[string]string) bool {
	if len(child) > len(farther) {
		return false
	}
	for k, v := range child {
		if farther[k] != v {
			return false
		}
	}
	return true
}

func CopyMap(m1 map[string]string) map[string]string {
	ret := make(map[string]string)
	for k, v := range m1 {
		ret[k] = v
	}
	return ret
}

func MapToMapInterface(m map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		ks := strings.Split(k, ".")
		r, ks, err := findSubNode(ks, out)
		if err != nil {
			fmt.Printf("===err:%s", err.Error())
		}
		for k1, v1 := range createSubmap(ks, v) {
			r[k1] = v1
		}
	}
	return out
}

func createSubmap(ks []string, value string) map[string]interface{} {
	if len(ks) == 1 {
		return map[string]interface{}{
			ks[0]: value,
		}
	}
	return map[string]interface{}{
		ks[0]: createSubmap(ks[1:], value),
	}
}

func findSubNode(ks []string, root map[string]interface{}) (map[string]interface{}, []string, error) {
	if len(ks) == 0 {
		return root, ks, nil
	}
	v, ok := root[ks[0]]
	if !ok {
		return root, ks, nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, ks, fmt.Errorf("leaf node reached,%v", ks)
	}
	return findSubNode(ks[1:], m)
}

// Subscribeable map
type SubcribeableMap[V any] struct {
	data           cmap.ConcurrentMap[string, V]
	subscriber     []func(key string, value interface{})
	subscriberLock sync.RWMutex
}

func NewSubcribeableMap[V any]() *SubcribeableMap[V] {
	return &SubcribeableMap[V]{
		data:           cmap.New[V](),
		subscriber:     make([]func(key string, value interface{}), 0),
		subscriberLock: sync.RWMutex{},
	}
}

func (s *SubcribeableMap[V]) Set(key string, v V) {
	s.data.Set(key, v)
	s.subscriberLock.RLock()
	for _, f := range s.subscriber {
		f(key, v)
	}
	s.subscriberLock.RUnlock()
}

func (s *SubcribeableMap[V]) Pop(key string) {
	s.data.Pop(key)
	s.subscriberLock.RLock()
	for _, f := range s.subscriber {
		f(key, nil)
	}
	s.subscriberLock.RUnlock()
}

func (s *SubcribeableMap[V]) Get(host string) V {
	if i, ok := s.data.Get(host); ok {
		return i
	}
	return *new(V)
}

func (s *SubcribeableMap[V]) Subscribe(subscribe func(key string, value interface{})) {
	s.subscriberLock.Lock()
	s.subscriber = append(s.subscriber, subscribe)
	s.subscriberLock.Unlock()
}
