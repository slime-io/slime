package module

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	istioapi "slime.io/slime/framework/apis"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/framework/model/pkg/leaderelection"
	"slime.io/slime/modules/limiter/api/config"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/controllers"
	"slime.io/slime/modules/limiter/model"
)

type Module struct {
	config config.Limiter
	env    bootstrap.Environment
	pc     *metric.ProducerConfig
	sr     *controllers.SmartLimiterReconciler
}

func (m *Module) Kind() string {
	return model.ModuleName
}

func (m *Module) Config() proto.Message {
	return &m.config
}

func (m *Module) InitScheme(scheme *runtime.Scheme) error {
	for _, f := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		microservicev1alpha2.AddToScheme,
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
	if err := m.init(opts.Env); err != nil {
		return err
	}

	if err := m.setupWithManager(opts.Manager); err != nil {
		return err
	}

	if err := m.setupWithLeaderElection(opts.LeaderElectionCbs); err != nil {
		return err
	}
	return nil
}

func (m *Module) init(env bootstrap.Environment) error {
	m.env = env
	pc, err := controllers.NewProducerConfig(m.env, &m.config)
	if err != nil {
		return err
	}
	m.pc = pc
	source := metric.NewSource(m.pc)
	m.sr = controllers.NewReconciler(
		controllers.ReconcilerWithCfg(&m.config),
		controllers.ReconcilerWithEnv(m.env),
		controllers.ReconcilerWithProducerConfig(m.pc),
		controllers.ReconcilerWithSource(source),
	)
	return nil
}

func (m *Module) setupWithManager(mgr manager.Manager) error {
	m.sr.Client = mgr.GetClient()
	m.sr.Scheme = mgr.GetScheme()

	if err := m.sr.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller SmartLimiter, %+v", err)
	}
	return nil
}

func (m *Module) setupWithLeaderElection(le leaderelection.LeaderCallbacks) error {
	le.AddOnStartedLeading(func(ctx context.Context) {
		log.Infof("producers starts")
		metric.NewProducer(m.pc, m.sr.Source)

		go m.sr.WatchMetric(ctx)
	})

	le.AddOnStoppedLeading(m.sr.Clear)
	return nil
}
