package module

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
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
	if config == nil {
		panic(fmt.Errorf("module config nil for %s", bundle))
	}
	err = util.InitLog(config.Global.Log)
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
			if config == nil {
				panic(fmt.Errorf("module config nil for %s", mod.Name))
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

	clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		log.Errorf("create a new clientSet failed, %+v", err)
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		log.Errorf("create a new dynamic client failed, %+v", err)
		os.Exit(1)
	}

	var startups []func(ctx context.Context)
	cbs := InitCallbacks{
		AddStartup: func(f func(ctx context.Context)) {
			startups = append(startups, f)
		},
	}

	ctx := context.Background()

	ph := bootstrap.NewPathHandler()

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

		modSelfCfg := mod.Config()
		if modCfg != nil && modSelfCfg != nil {

			var toCopy proto.Message
			switch {
			case modCfg.Fence != nil:
				toCopy = modCfg.Fence
			case modCfg.Limiter != nil:
				toCopy = modCfg.Limiter
			case modCfg.Plugin != nil:
				toCopy = modCfg.Plugin
			}

			if toCopy != nil {
				// old version: get mod.Config() value from config.Fence/Limiter/Plugin
				bs, err := proto.Marshal(toCopy)
				if err != nil {
					log.Errorf("marshal for mod %s config (%v) met err %v", mod.Name(), toCopy, err)
					fatal()
				}
				if err = proto.Unmarshal(bs, modSelfCfg); err != nil {
					log.Errorf("unmarshal for mod %s modSelfCfg (%v) met err %v", mod.Name(), bs, err)
					fatal()
				}
			} else {
				// new version: get mod.Config() value from config.general
				if modCfg.General != nil {
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

		}

		env := bootstrap.Environment{
			Config:        modCfg,
			K8SClient:     clientSet,
			DynamicClient: dynamicClient,
			HttpPathHandler: bootstrap.PrefixPathHandlerManager{
				Prefix:      mod.Name(),
				PathHandler: ph,
			},
			Stop: ctx.Done(),
		}

		if err := mod.InitManager(mgr, env, cbs); err != nil {
			log.Errorf("mod %s InitManager met err %v", mod.Name(), err)
			fatal()
		}
	}

	go bootstrap.AuxiliaryHttpServerStart(ph, config.Global.Misc["aux-addr"])

	for _, startup := range startups {
		startup(ctx)
	}

	log.Infof("starting manager bundle %s with modules %v", bundle, modNames)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Errorf("problem running manager, %+v", err)
		os.Exit(1)
	}
}
