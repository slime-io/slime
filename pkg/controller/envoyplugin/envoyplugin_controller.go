package envoyplugin

import (
	"context"

	mircroservice "slime.io/slime/pkg/apis/microservice/v1alpha1"
	"slime.io/slime/pkg/apis/microservice/v1alpha1/wrapper"
	"slime.io/slime/pkg/apis/networking/v1alpha3"
	"slime.io/slime/pkg/bootstrap"
	"slime.io/slime/pkg/util"

	istio "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	controller2 "slime.io/slime/pkg/controller"
)

var log = logf.Log.WithName("controller_envoyplugin")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new EnvoyPlugin Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, env *bootstrap.Environment) error {
	return add(mgr, newReconciler(mgr, env))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, env *bootstrap.Environment) reconcile.Reconciler {
	return &ReconcileEnvoyPlugin{client: mgr.GetClient(), scheme: mgr.GetScheme(), env: env}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("envoyplugin-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource EnvoyPlugin
	err = c.Watch(&source.Kind{Type: &wrapper.EnvoyPlugin{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileEnvoyPlugin implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileEnvoyPlugin{}

// ReconcileEnvoyPlugin reconciles a EnvoyPlugin object
type ReconcileEnvoyPlugin struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	env *bootstrap.Environment
}

// Reconcile reads that state of the cluster for a EnvoyPlugin object and makes changes based on the state read
// and what is in the EnvoyPlugin.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileEnvoyPlugin) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling EnvoyPlugin")

	// Fetch the EnvoyPlugin instance
	instance := &wrapper.EnvoyPlugin{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// 资源删除
	if err != nil && errors.IsNotFound(err) {
		for _, f := range controller2.DeleteHook[controller2.EnvoyPlugin] {
			if err := f(request, r); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// 资源更新
	for _, f := range controller2.UpdateHook[controller2.EnvoyPlugin] {
		if err := f(instance, r); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func DoUpdate(i metav1.Object, args ...interface{}) error {
	if len(args) == 0 {
		log.Error(nil, "envoyplugin doUpdate方法参数不足")
		return nil
	}
	if this, ok := args[0].(*ReconcileEnvoyPlugin); !ok {
		log.Error(nil, "envoyplugin doUpdate方法参数不足")
	} else {
		if instance, ok := i.(*wrapper.EnvoyPlugin); !ok {
			log.Error(nil, "envoyplugin doRemove方法第一参数需为自身")
		} else {
			ef := newEnvoyFilterForEnvoyPlugin(instance)
			if ef == nil {
				return nil
			}

			// 测试需要
			if this.scheme != nil {
				// Set EnvoyPlugin instance as the owner and controller
				if err := controllerutil.SetControllerReference(instance, ef, this.scheme); err != nil {
					return err
				}
			}

			found := &v1alpha3.EnvoyFilter{}
			err := this.client.Get(context.TODO(), types.NamespacedName{Name: ef.Name, Namespace: ef.Namespace}, found)
			if err != nil && errors.IsNotFound(err) {
				log.Info("Creating a new EnvoyFilter", "EnvoyFilter.Namespace", ef.Namespace, "EnvoyFilter.Name", ef.Name)
				err = this.client.Create(context.TODO(), ef)
				if err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				log.Info("Update a EnvoyFilter", "EnvoyFilter.Namespace", ef.Namespace, "EnvoyFilter.Name", ef.Name)
				ef.ResourceVersion = found.ResourceVersion
				err := this.client.Update(context.TODO(), ef)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func newEnvoyFilterForEnvoyPlugin(cr *wrapper.EnvoyPlugin) *v1alpha3.EnvoyFilter {
	pb, err := util.FromJSONMap("netease.microservice.v1alpha1.EnvoyPlugin", cr.Spec)
	if err != nil {
		log.Error(err, "unable to convert envoyPlugin to envoyFilter")
		return nil
	}
	envoyFilter := &istio.EnvoyFilter{}
	translateEnvoyPlugin(pb.(*mircroservice.EnvoyPlugin), envoyFilter)
	envoyFilterWrapper := &v1alpha3.EnvoyFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
	}
	if m, err := util.ProtoToMap(envoyFilter); err == nil {
		envoyFilterWrapper.Spec = m
		return envoyFilterWrapper
	}
	return nil
}
