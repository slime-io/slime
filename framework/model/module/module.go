package module

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gogo/protobuf/jsonpb"
	"os"
	"strings"

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

type moduleConfig struct {
	module Module
	config configH
}

type configH struct {
	config      *bootconfig.Config
	generalJson []byte
}

type Module interface {
	Kind() string
	Config() proto.Message
	InitScheme(scheme *runtime.Scheme) error
	InitManager(mgr manager.Manager, env bootstrap.Environment, cbs InitCallbacks) error
	Clone() Module
}

func Main(bundle string, modules []Module) {

	fatal := func() {
		os.Exit(1)
	}

	// prepare module definition map
	moduleDefinitions := make(map[string]Module)
	for _, mod := range modules {
		moduleDefinitions[mod.Kind()] = mod
	}

	// Init module of instance
	var mcs []*moduleConfig

	// get main module config
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

	// check if main module is bundle or not
	isBundle := config.Bundle != nil
	if !isBundle {
		var m Module
		if config.Kind != "" {
			m = moduleDefinitions[config.Kind]
		} else {
			// compatible for old version without kind field
			m = moduleDefinitions[config.Name]
		}
		if m == nil {
			log.Errorf("wrong kind or name of module %s", config.Name)
			fatal()
		}
		mc := &moduleConfig{
			module: m.Clone(),
			config: configH{
				config:      config,
				generalJson: generalJson,
			},
		}
		mcs = append(mcs, mc)
	} else {
		for _, modCfg := range config.Bundle.Modules {
			var m Module
			if modCfg.Kind != "" {
				m = moduleDefinitions[modCfg.Kind]
			} else {
				// compatible for old version without kind field
				m = moduleDefinitions[modCfg.Name]
			}
			if m == nil {
				log.Errorf("wrong kind or name of module %s", modCfg.Name)
				fatal()
			}

			modConfig, modRawCfg, modGeneralJson, err := bootstrap.GetModuleConfig(modCfg.Name)
			if err != nil {
				panic(err)
			}
			if modConfig == nil {
				modConfig = &bootconfig.Config{}
			}

			if config.Global != nil {
				if modConfig.Global == nil {
					modConfig.Global = &bootconfig.Global{}
				}
				modConfig.Global = merge(config.Global, modConfig.Global).(*bootconfig.Global)
			}
			log.Infof("load raw module config of bundle item %s: %s", modCfg.Name, string(modRawCfg))
			log.Infof("general config of bundle item %s: %s", modCfg.Name, string(modGeneralJson))
			modConfigJson, _ := json.Marshal(*modConfig)
			log.Infof("after merge with bundle config, module config of bundle item %s: %s", modCfg.Name, string(modConfigJson))

			mc := &moduleConfig{
				module: m.Clone(),
				config: configH{
					config:      modConfig,
					generalJson: modGeneralJson,
				},
			}
			mcs = append(mcs, mc)
		}
	}

	var (
		scheme   = runtime.NewScheme()
		modKinds []string
	)
	for _, mc := range mcs {
		modKinds = append(modKinds, mc.module.Kind())
		if err := mc.module.InitScheme(scheme); err != nil {
			log.Errorf("mod %s InitScheme met err %v", mc.module.Kind(), err)
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

	// init modules
	for _, mc := range mcs {
		modCfg, modGeneralJson := mc.config.config, mc.config.generalJson
		modSelfCfg := mc.module.Config()

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
				ma := jsonpb.Marshaler{}
				js, err := ma.MarshalToString(toCopy)
				if err != nil {
					log.Errorf("marshal for mod %s config (%v) met err %v", modCfg.Name, toCopy, err)
					fatal()
				}
				um := jsonpb.Unmarshaler{AllowUnknownFields: true}
				if err := um.Unmarshal(strings.NewReader(js), modSelfCfg); err != nil {
					log.Errorf("unmarshal for mod %s config (%v) met err %v", modCfg.Name, modGeneralJson, err)
					fatal()
				}
			} else {
				// new version: get mod.Config() value from config.general
				if modCfg.General != nil {
					if len(modCfg.General.XXX_unrecognized) > 0 {
						if err := proto.Unmarshal(modCfg.General.XXX_unrecognized, modSelfCfg); err != nil {
							log.Errorf("unmarshal for mod %s XXX_unrecognized (%v) met err %v", modCfg.Name, modCfg.General.XXX_unrecognized, err)
							fatal()
						}
					} else if len(modGeneralJson) > 0 {
						u := jsonpb.Unmarshaler{AllowUnknownFields: true}
						if err := u.Unmarshal(bytes.NewBuffer(modGeneralJson), modSelfCfg); err != nil {
							log.Errorf("unmarshal for mod %s modGeneralJson (%v) met err %v", modCfg.Name, modGeneralJson, err)
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
				Prefix:      modCfg.Name,
				PathHandler: ph,
			},
			Stop: ctx.Done(),
		}

		if err := mc.module.InitManager(mgr, env, cbs); err != nil {
			log.Errorf("mod %s InitManager met err %v", modCfg.Name, err)
			fatal()
		}
	}

	go bootstrap.AuxiliaryHttpServerStart(ph, config.Global.Misc["aux-addr"])

	for _, startup := range startups {
		startup(ctx)
	}

	log.Infof("starting manager bundle %s with modules %v", bundle, modKinds)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Errorf("problem running manager, %+v", err)
		os.Exit(1)
	}
}

// Merge The content of dst will not be changed, return a new instance with merged result
func merge(dst, src proto.Message) proto.Message {
	ret := proto.Clone(dst)
	proto.Merge(ret, src)
	return ret
}
