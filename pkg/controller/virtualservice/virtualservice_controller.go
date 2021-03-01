/*
* @Author: yangdihang
* @Date: 2020/5/21
 */

package virtualservice

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	networkingv1alpha3 "slime.io/slime/pkg/apis/networking/v1alpha3"
	"slime.io/slime/pkg/bootstrap"
	controller2 "slime.io/slime/pkg/controller"
)

var log = logf.Log.WithName("controller_virtualservice")
var DeleteHook = make([]func(reconcile.Request, ...interface{}), 0)
var UpdateHook = make([]func(networkingv1alpha3.VirtualService, ...interface{}), 0)
var AddHook = make([]func(networkingv1alpha3.VirtualService, ...interface{}), 0)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new VirtualService Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, env *bootstrap.Environment) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileVirtualService{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("virtualservice-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	// Watch for changes to primary resource VirtualService
	err = c.Watch(&source.Kind{Type: &networkingv1alpha3.VirtualService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

// blank assignment to verify that ReconcileVirtualService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileVirtualService{}

// ReconcileVirtualService reconciles a VirtualService object
type ReconcileVirtualService struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a VirtualService object and makes changes based on the state read
// and what is in the VirtualService.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileVirtualService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling VirtualService")

	// Fetch the VirtualService instance
	instance := &networkingv1alpha3.VirtualService{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)

	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// 资源删除
	if err != nil && errors.IsNotFound(err) {
		for _, f := range controller2.DeleteHook[controller2.VirtualService] {
			if err := f(request); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// 资源更新

	for _, f := range controller2.UpdateHook[controller2.VirtualService] {
		if err := f(instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func ServiceFenceProcess(i v1.Object, args ...interface{}) error {
	if instance, ok := i.(*networkingv1alpha3.VirtualService); ok {
		m := parseDestination(instance)
		for k, v := range m {
			HostDestinationMapping.Set(k, v)
		}
	}
	return nil
}

func parseDestination(instance *networkingv1alpha3.VirtualService) map[string][]string {
	ret := make(map[string][]string)

	hosts := make([]string, 0)
	i, ok := instance.Spec["hosts"].([]interface{})
	for _, iv := range i {
		hosts = append(hosts, iv.(string))
	}
	if !ok {
		return nil
	}

	dhs := make(map[string]struct{}, 0)

	if httpRoutes, ok := instance.Spec["http"].([]interface{}); ok {
		for _, httpRoute := range httpRoutes {
			if hr, ok := httpRoute.(map[string]interface{}); ok {
				if ds, ok := hr["route"].([]interface{}); ok {
					for _, d := range ds {
						if route, ok := d.(map[string]interface{}); ok {
							destinationHost := route["destination"].(map[string]interface{})["host"].(string)
							dhs[destinationHost] = struct{}{}
						}
					}
				}
			}
		}
	}

	for _, h := range hosts {
		for dh := range dhs {
			if h != dh {
				if ret[h] == nil {
					ret[h] = []string{dh}
				} else {
					ret[h] = append(ret[h], dh)
				}
			}
		}
	}
	return ret
}
