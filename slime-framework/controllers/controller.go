package controllers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type SetupAware interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// RegisterObjectReconciler is a shortcut to register reconciler for a specific api type.
// Especially caller can use reconcile.Func to fastly convert a callback to a reconciler impl.
func RegisterObjectReconciler(apiType runtime.Object, r reconcile.Reconciler, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(apiType).
		Complete(r)
}

type ObjectReconcileItem struct {
	Name     string
	ApiType  runtime.Object
	R        reconcile.Reconciler
	Mgr      ctrl.Manager
	Optional bool
}

type ObjectReconcilerBuilder struct {
	items []ObjectReconcileItem
}

func (b ObjectReconcilerBuilder) Add(item ObjectReconcileItem) ObjectReconcilerBuilder {
	return ObjectReconcilerBuilder{items: append(b.items, item)}
}

func (b ObjectReconcilerBuilder) Build(mgr ctrl.Manager) error {
	checkItem := func(idx int, item ObjectReconcileItem) error {
		if item.Mgr == nil && mgr == nil {
			return fmt.Errorf("item %s of idx %d has nil Mgr but build with nil mgr", item.Name, idx)
		}

		if item.R == nil {
			return fmt.Errorf("item %s of idx %d has nil R", item.Name, idx)
		}
		if item.ApiType == nil {
			if _, ok := item.R.(SetupAware); !ok {
				return fmt.Errorf("item %s of idx %d has nil ApiType but not SetupAware", item.Name, idx)
			}
		}

		return nil
	}

	processItem := func(idx int, item ObjectReconcileItem) error {
		m := item.Mgr
		if m == nil {
			m = mgr
		}

		if s, ok := item.R.(SetupAware); ok {
			if err := s.SetupWithManager(m); err != nil {
				return fmt.Errorf("do SetupWithManager for %s of idx %d met err %v", item.Name, idx, err)
			}
		} else if err := RegisterObjectReconciler(item.ApiType, item.R, m); err != nil {
			return fmt.Errorf("do RegisterObjectReconciler for %s of idx %d met err %v", item.Name, idx, err)
		}

		return nil
	}

	for idx, item := range b.items {
		if err := checkItem(idx, item); err != nil {
			if item.Optional {
				// TODO add log
			} else {
				return err
			}
		}
	}

	for idx, item := range b.items {
		if err := processItem(idx, item); err != nil {
			if item.Optional {
				// TODO add log
			} else {
				return err
			}
		}
	}

	return nil
}
