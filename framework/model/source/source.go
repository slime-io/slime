/*
* @Author: yangdihang
* @Date: 2020/10/13
 */

package source

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"slime.io/slime/framework/model"
)

type Source interface {
	Start(stop <-chan struct{})
	WatchAdd(meta types.NamespacedName)
	WatchRemove(meta types.NamespacedName)
	Get(meta types.NamespacedName) map[string]string
}

type EventType uint32

const (
	Add EventType = iota
	Delete
	Update
)

type Event struct {
	EventType
	Loc  types.NamespacedName
	Info map[string]string
}

type MetricSourceForModule interface {
	InterestAdd(nn types.NamespacedName)
	InterestRemove(nn types.NamespacedName)
}

type MetricSourceForAggregate interface {
	Name() string
	RestConfig() rest.Config
	GVKs() []schema.GroupVersionKind
	Notify(we model.WatcherEvent)
}

type AggregateMetricSource interface {
	GVKStopChanMap() map[schema.GroupVersionKind]chan bool
	RestConfig() *rest.Config
}
