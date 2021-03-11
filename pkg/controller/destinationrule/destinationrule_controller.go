package destinationrule

import (
	"context"

	corev1 "k8s.io/api/core/v1"
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

	istionetworking "istio.io/api/networking/v1alpha3"

	networkingv1alpha3 "slime.io/slime/pkg/apis/networking/v1alpha3"
	"slime.io/slime/pkg/bootstrap"
	controller2 "slime.io/slime/pkg/controller"
	"slime.io/slime/pkg/util"
)

var log = logf.Log.WithName("controller_destinationrule")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new DestinationRule Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, env *bootstrap.Environment) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDestinationRule{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("destinationrule-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource DestinationRule
	err = c.Watch(&source.Kind{Type: &networkingv1alpha3.DestinationRule{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner DestinationRule
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &networkingv1alpha3.DestinationRule{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileDestinationRule implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDestinationRule{}

// ReconcileDestinationRule reconciles a DestinationRule object
type ReconcileDestinationRule struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a DestinationRule object and makes changes based on the state read
// and what is in the DestinationRule.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDestinationRule) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling DestinationRule")

	// Fetch the VirtualService instance
	instance := &networkingv1alpha3.DestinationRule{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)

	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// 资源删除
	if err != nil && errors.IsNotFound(err) {
		for _, f := range controller2.DeleteHook[controller2.DestinationRule] {
			if err := f(request); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// 资源更新
	for _, f := range controller2.UpdateHook[controller2.DestinationRule] {
		if err := f(instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func DoUpdate(i v1.Object, args ...interface{}) error {
	if instance, ok := i.(*networkingv1alpha3.DestinationRule); ok {
		pb, err := util.FromJSONMap("istio.networking.v1alpha3.DestinationRule", instance.Spec)
		if err != nil {
			return err
		}
		if dr, ok := pb.(*istionetworking.DestinationRule); ok {
			drHost := util.UnityHost(dr.Host, instance.Namespace)
			HostSubsetMapping.Set(drHost, dr.Subsets)
		}
	}
	return nil
}
