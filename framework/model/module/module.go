package module

import (
	"bytes"
	"context"
	"os"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/util"
)

type InitCallbacks struct {
	AddStartup func(func(ctx context.Context))
}

type Module interface {
	Name() string
	Config() proto.Message
	InitScheme(scheme *runtime.Scheme) error
	InitManager(mgr manager.Manager, env bootstrap.Environment, cbs InitCallbacks) error
}

func Main(bundle string, modules []Module) {
	fatal := func() {
		os.Exit(1)
	}

	type configH struct {
		config      *bootconfig.Config
		generalJson []byte
	}

	config, rawCfg, generalJson, err := bootstrap.GetModuleConfig("")
	if err != nil {
		panic(err)
	}
	err = util.InitLog(config.Global.Log.LogLevel, config.Global.Log.KlogLevel)
	if err != nil {
		panic(err)
	}

	log.Infof("load module config of %s: %s", bundle, string(rawCfg))
	modConfigs := map[string]configH{}
	isBundle := config.Bundle != nil
	if isBundle {
		for _, mod := range config.Bundle.Modules {
			modConfig, modRawCfg, modGeneralJson, err := bootstrap.GetModuleConfig(mod.Name)
			if err != nil {
				panic(err)
			}

			if config.Global != nil {
				if modConfig.Global == nil {
					modConfig.Global = &bootconfig.Global{}
				}
				proto.Merge(config.Global, modConfig.Global)
			}
			log.Infof("load module config of bundle item %s: %s", mod.Name, string(modRawCfg))
			modConfigs[mod.Name] = configH{modConfig, modGeneralJson}
		}
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

	client, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		log.Errorf("create a new clientSet failed, %+v", err)
		os.Exit(1)
	}

	var startups []func(ctx context.Context)
	cbs := InitCallbacks{
		AddStartup: func(f func(ctx context.Context)) {
			startups = append(startups, f)
		},
	}

	ctx := context.Background()

	for _, mod := range modules {
		var modCfg *bootconfig.Config
		var modGeneralJson []byte
		if isBundle {
			if h, ok := modConfigs[mod.Name()]; ok {
				modCfg, modGeneralJson = h.config, h.generalJson
			}
		} else {
			modCfg, modGeneralJson = config, generalJson
		}

		if modCfg != nil && modCfg.General != nil {
			modSelfCfg := mod.Config()
			if modSelfCfg != nil {
				if len(modCfg.General.XXX_unrecognized) > 0 {
					if err := proto.Unmarshal(modCfg.General.XXX_unrecognized, modSelfCfg); err != nil {
						log.Errorf("unmarshal for mod %s XXX_unrecognized (%v) met err %v", mod.Name(), modCfg.General.XXX_unrecognized, err)
						fatal()
					}
				} else if len(modGeneralJson) > 0 {
					if err := jsonpb.Unmarshal(bytes.NewBuffer(modGeneralJson), modSelfCfg); err != nil {
						log.Errorf("unmarshal for mod %s modGeneralJson (%v) met err %v", mod.Name(), modGeneralJson, err)
						fatal()
					}
				}
			}
		}

		env := bootstrap.Environment{
			Config:    modCfg,
			K8SClient: client,
			Stop:      ctx.Done(),
		}

		if err := mod.InitManager(mgr, env, cbs); err != nil {
			log.Errorf("mod %s InitManager met err %v", mod.Name(), err)
			fatal()
		}
	}

	go bootstrap.AuxiliaryHttpServerStart(config.Global.Misc["aux-addr"])

	for _, startup := range startups {
		startup(ctx)
	}

	log.Infof("starting manager bundle %s with modules %v", bundle, modNames)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Errorf("problem running manager, %+v", err)
		os.Exit(1)
	}
}
