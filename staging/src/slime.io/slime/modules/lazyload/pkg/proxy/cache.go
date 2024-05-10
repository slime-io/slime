package proxy

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type Cache struct {
	Data map[types.NamespacedName]struct{}
	sync.RWMutex
}

func (c *Cache) Exist(nn types.NamespacedName) bool {
	c.RLock()
	defer c.RUnlock()
	_, ok := c.Data[nn]
	return ok
}

func (c *Cache) Set(nn types.NamespacedName) {
	c.Lock()
	defer c.Unlock()
	c.Data[nn] = struct{}{}
}

func (c *Cache) Delete(nn types.NamespacedName) {
	c.Lock()
	defer c.Unlock()
	delete(c.Data, nn)
}
