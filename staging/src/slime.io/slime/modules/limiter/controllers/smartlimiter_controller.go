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
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	"slime.io/slime/framework/util"

	cmap "github.com/orcaman/concurrent-map"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"slime.io/slime/framework/bootstrap"
	slime_model "slime.io/slime/framework/model"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/trigger"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

// SmartLimiterReconciler reconciles a SmartLimiter object
type SmartLimiterReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	cfg    *microservicev1alpha2.Limiter
	env    bootstrap.Environment
	scheme *runtime.Scheme
	// key is limiter's namespace and name
	// value is the host
	// use service.ns.svc.cluster.local as default if host not specified
	// or use mock host when limiter in gw and outbound
	interest cmap.ConcurrentMap
	// reuse, or use another filed to store interested nn
	// key is the interested namespace/name
	// value is the metricInfo
	metricInfo   cmap.ConcurrentMap
	MetricSource source.Source

	metricInfoLock sync.RWMutex

	lastUpdatePolicy     microservicev1alpha2.SmartLimiterSpec
	lastUpdatePolicyLock *sync.RWMutex

	watcherMetricChan <-chan metric.Metric
	tickerMetricChan  <-chan metric.Metric
}

// +kubebuilder:rbac:groups=microservice.slime.io,resources=smartlimiters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io,resources=smartlimiters/status,verbs=get;update;patch

func (r *SmartLimiterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Infof("begin reconcile, get smartlimiter %+v", req)
	instance := &microservicev1alpha2.SmartLimiter{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			instance = nil
			err = nil
			log.Infof("smartlimiter %v not found", req.NamespacedName)
		} else {
			log.Errorf("get smartlimiter %v err, %s", req.NamespacedName, err)
			return reconcile.Result{}, err
		}
	}
	// deleted
	if instance == nil {
		r.lastUpdatePolicyLock.Lock()
		r.lastUpdatePolicy = microservicev1alpha2.SmartLimiterSpec{}
		r.lastUpdatePolicyLock.Unlock()
		r.RemoveInterested(req)
		log.Infof("%v is deleted", req)
		return reconcile.Result{}, nil
	} else {
		// add or update
		log.Infof("%v is added or updated", req)
		if !r.env.RevInScope(slime_model.IstioRevFromLabel(instance.Labels)) {
			log.Debugf("existing smartlimiter %v istiorev %s but our %s, skip ...",
				req.NamespacedName, slime_model.IstioRevFromLabel(instance.Labels), r.env.IstioRev())
			return ctrl.Result{}, nil
		}
		r.lastUpdatePolicyLock.RLock()
		if reflect.DeepEqual(instance.Spec, r.lastUpdatePolicy) {
			r.lastUpdatePolicyLock.RUnlock()
			return reconcile.Result{}, nil
		} else {
			if out, err := r.Validate(instance); err != nil {
				log.Errorf("invalid smartlimiter, %s", err)
				return reconcile.Result{}, nil
			} else {
				r.lastUpdatePolicyLock.RUnlock()
				r.lastUpdatePolicyLock.Lock()
				r.lastUpdatePolicy = instance.Spec
				r.lastUpdatePolicyLock.Unlock()
				r.RegisterInterest(instance, req, out)
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *SmartLimiterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microservicev1alpha2.SmartLimiter{}).
		Complete(r)
}

func NewReconciler(mgr ctrl.Manager, env bootstrap.Environment, cfg *microservicev1alpha2.Limiter) *SmartLimiterReconciler {
	r := &SmartLimiterReconciler{
		cfg:                  cfg,
		Client:               mgr.GetClient(),
		scheme:               mgr.GetScheme(),
		metricInfo:           cmap.New(),
		interest:             cmap.New(),
		env:                  env,
		lastUpdatePolicyLock: &sync.RWMutex{},
	}

	pc, err := newProducerConfig(env, cfg)
	if err != nil {
		log.Errorf("new producer config err, %v", err)
		os.Exit(1)
	}
	r.watcherMetricChan = pc.WatcherProducerConfig.MetricChan
	r.tickerMetricChan = pc.TickerProducerConfig.MetricChan
	pc.WatcherProducerConfig.NeedUpdateMetricHandler = r.handleWatcherEvent
	pc.TickerProducerConfig.NeedUpdateMetricHandler = r.handleTickerEvent
	metric.NewProducer(pc)
	log.Infof("producers starts")

	go r.WatchMetric()
	return r
}

func newProducerConfig(env bootstrap.Environment, cfg *microservicev1alpha2.Limiter) (*metric.ProducerConfig, error) {
	pc := &metric.ProducerConfig{
		EnableWatcherProducer: false,
		WatcherProducerConfig: metric.WatcherProducerConfig{
			Name:       "smartLimiter-watcher",
			MetricChan: make(chan metric.Metric),
			WatcherTriggerConfig: trigger.WatcherTriggerConfig{
				Kinds: []schema.GroupVersionKind{
					{
						Group:   "",
						Version: "v1",
						Kind:    "Endpoints",
					},
				},
				EventChan:     make(chan trigger.WatcherEvent),
				DynamicClient: env.DynamicClient,
			},
		},
		EnableTickerProducer: true,
		TickerProducerConfig: metric.TickerProducerConfig{
			Name:       "smartLimiter-ticker",
			MetricChan: make(chan metric.Metric),
			TickerTriggerConfig: trigger.TickerTriggerConfig{
				Durations: []time.Duration{
					30 * time.Second,
				},
				EventChan: make(chan trigger.TickerEvent),
			},
		},
		StopChan: env.Stop,
	}

	if !cfg.GetDisableAdaptive() {
		log.Info("enable adaptive ratelimiter")
		prometheusSourceConfig, err := newPrometheusSourceConfig(env)
		if err != nil {
			return nil, err
		}
		log.Infof("create new prometheus client success")
		pc.PrometheusSourceConfig = prometheusSourceConfig
		pc.EnablePrometheusSource = true
		pc.EnableWatcherProducer = true
	} else {
		log.Info("disable adaptive ratelimiter and promql is closed")
		pc.EnableMockSource = true
	}
	return pc, nil
}

func (r *SmartLimiterReconciler) RegisterInterest(instance *microservicev1alpha2.SmartLimiter, req ctrl.Request, out bool) {
	key := FQN(req.Namespace, req.Name)

	// query hostname base on instance
	host := buildHost(instance, out)

	// set interest mapping
	r.interest.Set(key, host)
	log.Infof("set %s/%s = %s", instance.Namespace, instance.Name, host)
}

func (r *SmartLimiterReconciler) RemoveInterested(req ctrl.Request) {
	key := FQN(req.Namespace, req.Name)

	r.metricInfo.Pop(key)
	r.interest.Pop(key)
	log.Infof("name %s, namespace %s has poped", req.Name, req.Namespace)
	// if contain global smart limiter, should delete info in configmap
	if !r.cfg.GetDisableGlobalRateLimit() {
		log.Infof("refresh global rate limiter configmap")
		refreshConfigMap([]*model.Descriptor{}, r, req.NamespacedName)
	} else {
		log.Info("global rate limiter is closed")
	}
}

func (r *SmartLimiterReconciler) Validate(instance *microservicev1alpha2.SmartLimiter) (bool, error) {
	var out bool
	gw := instance.Spec.Gateway
	var target *microservicev1alpha2.Target

	for _, descriptors := range instance.Spec.Sets {
		for _, descriptor := range descriptors.Descriptor_ {

			if descriptor.Target == nil && instance.Spec.Target == nil {
				return out, fmt.Errorf("invalid target")
			}
			if descriptor.Action == nil {
				return out, fmt.Errorf("invalid action")
			}
			if descriptor.Condition == "" {
				return out, fmt.Errorf("invalid condition")
			}

			condition := descriptor.Condition
			direction := target.GetDirection()

			quota := descriptor.Action.Quota
			if gw || direction == model.Outbound {
				out = true
				if !r.cfg.GetDisableAdaptive() {
					return out, fmt.Errorf("outbound/gw must disable adaptive limiter")
				}

				if condition != "true" {
					return out, fmt.Errorf("condition must true in outbound")
				}
				if _, err := strconv.Atoi(quota); err != nil {
					return out, fmt.Errorf("quota must be number in outbound")
				}
			}
		}
	}
	return out, nil
}

func FQN(ns, name string) string {
	return ns + "/" + name
}

func buildHost(instance *microservicev1alpha2.SmartLimiter, out bool) string {
	// get name.namespaces.svc.cluster.local by default
	host := util.UnityHost(instance.Name, instance.Namespace)

	if out {
		host = model.MockHost
	} else if instance.Spec.Host != "" {
		host = instance.Spec.Host
	}
	return host
}
