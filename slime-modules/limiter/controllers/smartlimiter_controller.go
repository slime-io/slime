/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"reflect"
	"sync"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"slime.io/slime/slime-framework/model/source/aggregate"
	"slime.io/slime/slime-framework/model/source/k8s"
	"slime.io/slime/slime-modules/limiter/controllers/multicluster"

	cmap "github.com/orcaman/concurrent-map"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"slime.io/slime/slime-framework/bootstrap"
	event_source "slime.io/slime/slime-framework/model/source"
	microserviceslimeiov1alpha1 "slime.io/slime/slime-modules/limiter/api/v1alpha1"
)

// SmartLimiterReconciler reconciles a SmartLimiter object
type SmartLimiterReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	env    *bootstrap.Environment
	scheme *runtime.Scheme

	metricInfo   cmap.ConcurrentMap
	MetricSource source.Source

	metricInfoLock sync.RWMutex
	eventChan      chan event_source.Event
	source         event_source.Source

	lastUpdatePolicy     microserviceslimeiov1alpha1.SmartLimiterSpec
	lastUpdatePolicyLock *sync.RWMutex
}

// +kubebuilder:rbac:groups=microservice.slime.io,resources=smartlimiters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io,resources=smartlimiters/status,verbs=get;update;patch

func (r *SmartLimiterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()

	// your logic here

	instance := &microserviceslimeiov1alpha1.SmartLimiter{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)

	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// 资源删除
	if err != nil && errors.IsNotFound(err) {
		log.Infof("metricInfo.Pop, name %s, namespace,%s", req.Name, req.Namespace)
		r.metricInfo.Pop(req.Namespace + "/" + req.Name)
		r.source.WatchRemove(req.NamespacedName)
		r.lastUpdatePolicyLock.Lock()
		r.lastUpdatePolicy = microserviceslimeiov1alpha1.SmartLimiterSpec{}
		r.lastUpdatePolicyLock.Unlock()
		return reconcile.Result{}, nil
	}

	// 资源更新
	r.lastUpdatePolicyLock.RLock()
	if reflect.DeepEqual(instance.Spec, r.lastUpdatePolicy) {
		r.lastUpdatePolicyLock.RUnlock()
		return reconcile.Result{}, nil
	} else {
		r.lastUpdatePolicyLock.RUnlock()
		r.lastUpdatePolicyLock.Lock()
		r.lastUpdatePolicy = instance.Spec
		r.lastUpdatePolicyLock.Unlock()
		r.source.WatchAdd(req.NamespacedName)
	}

	return ctrl.Result{}, nil
}

func (r *SmartLimiterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microserviceslimeiov1alpha1.SmartLimiter{}).
		Complete(r)
}

func NewReconciler(mgr ctrl.Manager, env *bootstrap.Environment) *SmartLimiterReconciler {
	log := log.WithField("controllers", "SmartLimiter")
	eventChan := make(chan event_source.Event)
	src := &aggregate.Source{}
	ms, err := k8s.NewMetricSource(eventChan, env)
	if err != nil {
		log.Errorf("failed to create slime-metric,%+v", err)
		return nil
	}
	src.AppendSource(ms)
	f := ms.SourceClusterHandler()
	if env.Config.Limiter.Multicluster {
		mc := multicluster.New(env, []func(*kubernetes.Clientset){f}, nil)
		go mc.Run()
	}
	r := &SmartLimiterReconciler{
		Client:               mgr.GetClient(),
		scheme:               mgr.GetScheme(),
		metricInfo:           cmap.New(),
		eventChan:            eventChan,
		source:               src,
		env:                  env,
		lastUpdatePolicyLock: &sync.RWMutex{},
	}
	r.source.Start(env.Stop)
	r.WatchSource(env.Stop)
	return r
}
