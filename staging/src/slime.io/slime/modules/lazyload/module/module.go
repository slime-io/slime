package module

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	istioapi "slime.io/slime/framework/apis"
	basecontroller "slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/modules/lazyload/api/config"
	lazyloadapiv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
	"slime.io/slime/modules/lazyload/controllers"
	modmodel "slime.io/slime/modules/lazyload/model"
)

var log = modmodel.ModuleLog

type Module struct {
	config config.Fence
}

func (m *Module) Kind() string {
	return modmodel.ModuleName
}

func (m *Module) Config() proto.Message {
	return &m.config
}

func (m *Module) InitScheme(scheme *runtime.Scheme) error {
	for _, f := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		lazyloadapiv1alpha1.AddToScheme,
		istioapi.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) Clone() module.Module {
	ret := *m
	return &ret
}

func (m *Module) Setup(opts module.ModuleOptions) error {

	log.Debugf("lazyload setup begin")
	env, mgr, le := opts.Env, opts.Manager, opts.LeaderElectionCbs
	pc, err := controllers.NewProducerConfig(env)
	if err != nil {
		return fmt.Errorf("unable to create ProducerConfig, %+v", err)
	}
	sfReconciler := controllers.NewReconciler(
		controllers.ReconcilerWithCfg(&m.config),
		controllers.ReconcilerWithEnv(env),
		controllers.ReconcilerWithProducerConfig(pc),
	)
	sfReconciler.Client = mgr.GetClient()
	sfReconciler.Scheme = mgr.GetScheme()

	opts.InitCbs.AddStartup(func(ctx context.Context) {
		sfReconciler.StartSvcCache(ctx)
		sfReconciler.StartIpToSvcCache(ctx)
	})

	var builder basecontroller.ObjectReconcilerBuilder

	// auto generate ServiceFence or not
	if m.config.AutoFence {
		builder = builder.Add(basecontroller.ObjectReconcileItem{
			Name:    "Namespace",
			ApiType: &corev1.Namespace{},
			R:       reconcile.Func(sfReconciler.ReconcileNamespace),
		}).Add(basecontroller.ObjectReconcileItem{
			Name:    "Service",
			ApiType: &corev1.Service{},
			R:       reconcile.Func(sfReconciler.ReconcileService),
		})

		podController := sfReconciler.NewPodController(env.K8SClient, m.config.FenceLabelKeyAlias)
		le.AddOnStartedLeading(func(ctx context.Context) {
			go podController.Run(ctx.Done())
		})

	}

	builder = builder.Add(basecontroller.ObjectReconcileItem{
		Name: "ServiceFence",
		R:    sfReconciler,
	}).Add(basecontroller.ObjectReconcileItem{
		Name: "VirtualService",
		R: &basecontroller.VirtualServiceReconciler{
			Env:    &env,
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
	})

	if err := builder.Build(mgr); err != nil {
		return fmt.Errorf("unable to create controller,%+v", err)
	}

	le.AddOnStartedLeading(func(_ context.Context) {
		log.Infof("producers starts")
		metric.NewProducer(pc)
	})
	if m.config.AutoPort {

		le.AddOnStartedLeading(func(ctx context.Context) {
			sfReconciler.StartAutoPort(ctx)
		})
	}

	if env.Config.Metric != nil ||
		env.Config.Global.Misc["metricSourceType"] == controllers.MetricSourceTypeAccesslog {
		le.AddOnStartedLeading(func(ctx context.Context) {
			go sfReconciler.WatchMetric(ctx)
		})
	} else {
		log.Warningf("watching metric is not running")
	}

	le.AddOnStoppedLeading(sfReconciler.Clear)
	return nil
}
