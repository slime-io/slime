/*
* @Author: yangdihang
* @Date: 2020/10/13
 */

package source

import "k8s.io/apimachinery/pkg/types"

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
