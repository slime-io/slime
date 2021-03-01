package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"slime.io/slime/pkg/bootstrap"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager, *bootstrap.Environment) error
var UpdateHook = make(map[Collection][]func(metav1.Object, ...interface{}) error)
var DeleteHook = make(map[Collection][]func(reconcile.Request, ...interface{}) error, 0)

type Collection int64

const (
	VirtualService Collection = iota
	DestinationRule
	ServiceFence
	PluginManager
	EnvoyPlugin
	SmartLimiter
	GatewayPlugin
	PilotAdmin
	TrafficMarkDestinationRule
	TrafficMarkEndpoints
	TrafficMarkHelm
)

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, env *bootstrap.Environment) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m, env); err != nil {
			return err
		}
	}
	return nil
}

type Worker interface {
	Refresh(request reconcile.Request, args map[string]string) (reconcile.Result, error)
	WatchSource(stop <-chan struct{})
}

type WorkEventType uint32

const (
	Add WorkEventType = iota
	Delete
	Update
)

type WorkEvent struct {
	WorkEventType
	Loc types.NamespacedName
}
