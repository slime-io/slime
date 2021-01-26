/*
* @Author: yangdihang
* @Date: 2020/5/21
 */

package virtualservice

import (
	"sync"

	"github.com/orcaman/concurrent-map"
)

var hostDestinationMapping cmap.ConcurrentMap
var Subscriber []func(string, []string)
var lock sync.RWMutex

func init() {
	if hostDestinationMapping == nil {
		hostDestinationMapping = cmap.New()
		Subscriber = make([]func(string, []string), 0)
		lock = sync.RWMutex{}
	}
}

func SetHostDestinationMapping(host string, destination []string) {
	hostDestinationMapping.Set(host, destination)
	lock.RLock()
	for _, f := range Subscriber {
		f(host, destination)
	}
	lock.RUnlock()
}

func GetHostDestinationMapping(host string) []string {
	if i, ok := hostDestinationMapping.Get(host); ok {
		if ret, ok := i.([]string); ok {
			return ret
		}
	}
	return nil
}

func Subscribe(subscribe func(host string, destination []string)) {
	lock.Lock()
	Subscriber = append(Subscriber, subscribe)
	lock.Unlock()
}
