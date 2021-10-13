package model

import (
	"context"
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"slime.io/slime/slime-framework/bootstrap"
	"slime.io/slime/slime-framework/util"
)

type ModuleInitCallbacks struct {
	AddStartup func(func(ctx context.Context))
}

type Module interface {
	Name() string
	InitScheme(scheme *runtime.Scheme) error
	InitManager(mgr manager.Manager, env bootstrap.Environment, cbs ModuleInitCallbacks) error
}

func Main(bundle string, modules []Module) {
	fatal := func() {
		os.Exit(1)
	}

	config := bootstrap.GetModuleConfig()
	err := util.InitLog(config.Global.Log.LogLevel, config.Global.Log.KlogLevel)
	if err != nil {
		panic(err.Error())
	}

	var (
		scheme   = runtime.NewScheme()
		modNames []string
	)
	for _, mod := range modules {
		modNames = append(modNames, mod.Name())
		if err := mod.InitScheme(scheme); err != nil {
			log.Errorf("mod %s InitScheme met err %v", mod.Name(), err)
			fatal()
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: config.Global.Misc["metrics-addr"],
		Port:               9443,
		LeaderElection:     config.Global.Misc["enable-leader-election"] == "on",
		LeaderElectionID:   bundle,
	})
	if err != nil {
		log.Errorf("unable to start manager %s, %+v", bundle, err)
		fatal()
	}

	env := bootstrap.Environment{}
	env.Config = config
	client, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		log.Errorf("create a new clientSet failed, %+v", err)
		os.Exit(1)
	}
	env.K8SClient = client

	var startups []func(ctx context.Context)
	cbs := ModuleInitCallbacks{
		AddStartup: func(f func(ctx context.Context)) {
			startups = append(startups, f)
		},
	}
	for _, mod := range modules {
		if err := mod.InitManager(mgr, env, cbs); err != nil {
			log.Errorf("mod %s InitManager met err %v", mod.Name(), err)
			fatal()
		}
	}

	go bootstrap.AuxiliaryHttpServerStart(config.Global.Misc["aux-addr"])

	ctx := context.Background()
	for _, startup := range startups {
		startup(ctx)
	}

	log.Info("starting manager bundle %s with modules %v", bundle, modNames)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Errorf("problem running manager, %+v", err)
		os.Exit(1)
	}
}
