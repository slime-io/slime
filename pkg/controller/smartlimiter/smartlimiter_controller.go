package smartlimiter

import (
	"context"
	"sync"

	cmap "github.com/orcaman/concurrent-map"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	microservicev1alpha1 "slime.io/slime/pkg/apis/microservice/v1alpha1"
	"slime.io/slime/pkg/bootstrap"
	controller2 "slime.io/slime/pkg/controller"
	"slime.io/slime/pkg/controller/smartlimiter/multicluster"
	event_source "slime.io/slime/pkg/model/source"
	"slime.io/slime/pkg/model/source/aggregate"
	"slime.io/slime/pkg/model/source/k8s"
)

const MATCH_KEY = "header_value_match"

var log = logf.Log.WithName("controller_smartlimiter")

func Add(mgr manager.Manager, env *bootstrap.Environment) error {
	r := newReconciler(mgr, env)
	err := add(mgr, r)
	if err != nil {
		return err
	}
	return nil
}

func newReconciler(mgr manager.Manager, env *bootstrap.Environment) reconcile.Reconciler {
	eventChan := make(chan event_source.Event)
	src := &aggregate.Source{}
	ms, err := k8s.NewMetricSource(eventChan, env)
	if err != nil {
		log.Error(err,"failed to create slime-metric")
		return nil
	}
	src.AppendSource(ms)
	f := ms.SourceClusterHandler()
	if env.Config.Limiter.Multicluster {
		mc := multicluster.New(env, []func(*kubernetes.Clientset){f}, nil)
		go mc.Run()
	}
	r := &ReconcileSmartLimiter{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		metricInfo: cmap.New(),
		eventChan:  eventChan,
		source:     src,
		env:        env,
	}
	r.source.Start(env.Stop)
	r.WatchSource(env.Stop)

	return r
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("smartlimiter-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource SmartLimiter
	err = c.Watch(&source.Kind{Type: &microservicev1alpha1.SmartLimiter{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileSmartLimiter implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileSmartLimiter{}

// ReconcileSmartLimiter reconciles a SmartLimiter object
type ReconcileSmartLimiter struct {
	env *bootstrap.Environment
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	metricInfo   cmap.ConcurrentMap
	MetricSource source.Source

	metricInfoLock sync.RWMutex
	eventChan      chan event_source.Event
	source         event_source.Source
}

func (r *ReconcileSmartLimiter) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling SmartLimiter")
	instance := &microservicev1alpha1.SmartLimiter{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)

	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// 资源删除
	if err != nil && errors.IsNotFound(err) {
		for _, f := range controller2.DeleteHook[controller2.SmartLimiter] {
			if err := f(request, r); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// 资源更新
	for _, f := range controller2.UpdateHook[controller2.SmartLimiter] {
		if err := f(instance, r); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func DoRemove(request reconcile.Request, args ...interface{}) error {
	if len(args) == 0 {
		log.Error(nil, "smartlimiter DoRemove方法参数不足")
		return nil
	}
	if this, ok := args[0].(*ReconcileSmartLimiter); !ok {
		log.Error(nil, "smartlimiter DoRemove方法第一参数需为自身")
	} else {
		this.metricInfo.Pop(request.Namespace + "/" + request.Name)
		this.source.WatchRemove(request.NamespacedName)
	}
	return nil
}

func DoUpdate(instance metav1.Object, args ...interface{}) error {
	r := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}
	if len(args) == 0 {
		log.Error(nil, "smartlimiter DoUpdate方法参数不足")
		return nil
	}
	if this, ok := args[0].(*ReconcileSmartLimiter); !ok {
		log.Error(nil, "smartlimiter DoUpdate方法第一参数需为自身")
	} else {
		this.source.WatchAdd(r)
	}
	return nil
}
