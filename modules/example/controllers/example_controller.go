package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/modules/example/api/config"
)

// ExampleReconciler reconciles a ... object
type ExampleReconciler struct {
	Cfg *config.Example
	client.Client
	Scheme *runtime.Scheme
	Env    *bootstrap.Environment
}

func (r *ExampleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *ExampleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(nil).
		Complete(r)
}
