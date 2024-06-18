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
	"sync"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"slime.io/slime/framework/bootstrap"
	slime_model "slime.io/slime/framework/model"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/trigger"
	"slime.io/slime/framework/util"
	"slime.io/slime/modules/limiter/api/config"
	limiterv1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

// SmartLimiterReconciler reconciles a SmartLimiter object
type SmartLimiterReconciler struct {
	client.Client
	sync.RWMutex
	Scheme *runtime.Scheme

	cfg *config.Limiter
	env bootstrap.Environment
	// key is limiter's namespace and name
	// value is the SmartLimiterMeta
	// subsequent code can get this information directly rather than through clientSet
	interest cmap.ConcurrentMap[string, SmartLimiterMeta]
	// reuse, or use another filed to store interested nn
	// key is the interested namespace/name
	// value is the metricInfo
	metricInfo cmap.ConcurrentMap[string, *slime_model.Endpoints]

	watcherMetricChan <-chan metric.Metric
	tickerMetricChan  <-chan metric.Metric
	Source            metric.Source
}

//nolint: lll
// +kubebuilder:rbac:groups=microservice.slime.io,resources=smartlimiters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io,resources=smartlimiters/status,verbs=get;update;patch

func (r *SmartLimiterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log.Infof("begin reconcile, get smartlimiter %+v", req)
	ReconcilesTotal.Increment()

	instance := &limiterv1alpha2.SmartLimiter{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			instance = nil
			log.Infof("smartlimiter %v not found", req.NamespacedName)
		} else {
			log.Errorf("get smartlimiter %v err, %s", req.NamespacedName, err)
			return reconcile.Result{}, err
		}
	}

	// deleted
	if instance == nil {
		r.RemoveInterested(req)
		log.Infof("%v is deleted", req)
		return reconcile.Result{}, nil
	}

	// add or update
	log.Infof("%v is added or updated", req)
	if !r.env.RevInScope(slime_model.IstioRevFromLabel(instance.Labels)) {
		log.Debugf("existing smartlimiter %v istiorev %s but our %s, skip ...",
			req.NamespacedName, slime_model.IstioRevFromLabel(instance.Labels), r.env.IstioRev())
		return ctrl.Result{}, nil
	}

	sidecarOutbound, gateway, err := r.Validate(instance)
	if err != nil {
		log.Errorf("invalid smartlimiter, %s", err)
		ValidationFailedTotal.Increment()
		return reconcile.Result{}, nil
	}
	r.RegisterInterest(instance, req, sidecarOutbound, gateway)

	log.Debugf("update start")
	r.RefreshResource(instance)

	return ctrl.Result{}, nil
}

func (r *SmartLimiterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&limiterv1alpha2.SmartLimiter{}).
		Complete(r)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(opts ...ReconcilerOpts) *SmartLimiterReconciler {
	r := &SmartLimiterReconciler{
		metricInfo: cmap.New[*slime_model.Endpoints](),
		interest:   cmap.New[SmartLimiterMeta](),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *SmartLimiterReconciler) Clear() {
	r.interest.Clear()
	r.metricInfo.Clear()
}

func NewProducerConfig(env bootstrap.Environment, cfg *config.Limiter) (*metric.ProducerConfig, error) {
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

func (r *SmartLimiterReconciler) RegisterInterest(
	instance *limiterv1alpha2.SmartLimiter,
	req ctrl.Request,
	sidecarOutbound bool,
	gateway bool,
) {
	// set interest mapping
	meta := SmartLimiterMeta{}
	meta.gateway = gateway
	meta.sidecarOutbound = sidecarOutbound
	meta.workloadSelector = instance.Spec.WorkloadSelector
	meta.seHost = instance.Spec.Host
	meta.host = util.UnityHost(instance.Name, instance.Namespace)

	key := FQN(req.Namespace, req.Name)
	r.interest.Set(key, meta)
	log.Infof("set %s/%s = %+v", instance.Namespace, instance.Name, meta)

	count := r.interest.Count()
	CachedLimiter.Record(float64(count))
}

type SmartLimiterMeta struct {
	host             string
	seHost           string
	workloadSelector map[string]string

	sidecarOutbound bool
	gateway         bool
}

func (r *SmartLimiterReconciler) RemoveInterested(req ctrl.Request) {
	key := FQN(req.Namespace, req.Name)

	r.metricInfo.Pop(key)
	r.interest.Pop(key)
	log.Infof("name %s, namespace %s has poped", req.Name, req.Namespace)
	count := r.interest.Count()
	CachedLimiter.Record(float64(count))
	// if contain global smart limiter, should delete info in configmap
	if !r.cfg.GetDisableGlobalRateLimit() {
		log.Infof("refresh global rate limiter configmap")
		refreshConfigMap([]*model.Descriptor{}, r, req.NamespacedName)
	} else {
		log.Info("global rate limiter is closed")
	}
}

func (r *SmartLimiterReconciler) Validate(instance *limiterv1alpha2.SmartLimiter) (bool, bool, error) {
	gateway := instance.Spec.Gateway
	sidecarOutbound := false

	for _, descriptors := range instance.Spec.Sets {
		for _, descriptor := range descriptors.Descriptor_ {
			target := instance.Spec.Target
			if descriptor.Target != nil {
				target = descriptor.Target
			}
			if target == nil {
				return sidecarOutbound, gateway, fmt.Errorf("invalid target")
			}
			if descriptor.Action == nil {
				return sidecarOutbound, gateway, fmt.Errorf("invalid action")
			}
			if descriptor.Condition == "" {
				return sidecarOutbound, gateway, fmt.Errorf("invalid condition")
			}

			condition := descriptor.Condition
			direction := target.GetDirection()

			if direction == model.Outbound {
				sidecarOutbound = true
			}

			if gateway || sidecarOutbound {
				if !r.cfg.GetDisableAdaptive() {
					return sidecarOutbound, gateway, fmt.Errorf("outbound/gw must disable adaptive limiter")
				}
				if condition != "true" {
					return sidecarOutbound, gateway, fmt.Errorf("condition must true in outbound/siddecar")
				}
			}
		}
	}
	return sidecarOutbound, gateway, nil
}

// RefreshResource refresh smartlimiter and ef on time
// even if reconcile fails, there are still ticker tasks to refresh
// only logging errors, no errors return
func (r *SmartLimiterReconciler) RefreshResource(instance *limiterv1alpha2.SmartLimiter) {
	log := log.WithField("function", "RefreshResource")

	queryMap := r.handleEvent(types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name,
	})
	if len(queryMap) == 0 {
		log.Debug("get empty queryMap, waiting for timed tasks")
		return
	}

	metric, err := r.Source.QueryMetric(queryMap)
	if err != nil {
		log.Errorf("query metric of %s/%s failed: %s", instance.Namespace, instance.Name, err.Error())
	} else if len(metric) == 0 {
		log.Errorf("query metric of %s/%s get empty", instance.Namespace, instance.Name)
	} else {
		log.Debugf("get metric %+v", metric)
		r.ConsumeMetric(metric)
	}
}

func FQN(ns, name string) string {
	return ns + "/" + name
}

type ReconcilerOpts func(reconciler *SmartLimiterReconciler)

func ReconcilerWithEnv(env bootstrap.Environment) ReconcilerOpts {
	return func(sr *SmartLimiterReconciler) {
		sr.env = env
	}
}

func ReconcilerWithCfg(cfg *config.Limiter) ReconcilerOpts {
	return func(sr *SmartLimiterReconciler) {
		sr.cfg = cfg
	}
}

func ReconcilerWithSource(source metric.Source) ReconcilerOpts {
	return func(sr *SmartLimiterReconciler) {
		sr.Source = source
	}
}

func ReconcilerWithProducerConfig(pc *metric.ProducerConfig) ReconcilerOpts {
	return func(sr *SmartLimiterReconciler) {
		sr.watcherMetricChan = pc.WatcherProducerConfig.MetricChan
		sr.tickerMetricChan = pc.TickerProducerConfig.MetricChan
		// reconciler defines producer metric handler
		pc.WatcherProducerConfig.NeedUpdateMetricHandler = sr.handleWatcherEvent
		pc.TickerProducerConfig.NeedUpdateMetricHandler = sr.handleTickerEvent
	}
}
