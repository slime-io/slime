package module

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlleaderelection "sigs.k8s.io/controller-runtime/pkg/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/pkg/leaderelection"
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
	// Init initialize the module according to the environment context.
	// It is not recommended to execute business logic or start resident
	// services at this stage, and it is forbidden to start resident
	// services that require a single instance to run at this stage.
	Init(env bootstrap.Environment) error
	// SetupWithInitCallbacks registers init callbacks that supports
	// concurrent execution, and notifies exit through `Context`, and
	// the callback must be non-blocking.
	SetupWithInitCallbacks(cbs InitCallbacks) error
	// SetupWithManager registers `Reconciler` with `Manager` to build a `Controller`
	SetupWithManager(mgr manager.Manager) error
	// SetupWithLeaderElection registers callbacks that require a single
	// instance to run.
	// Generally, resident services that run concurrently and trigger the
	// creation and update of resources in the cluster may involve race
	// conditions and cause system exceptions. The startup of these services
	// must be controlled through an election mechanism.
	SetupWithLeaderElection(le leaderelection.LeaderCallbacks) error
	Clone() Module
}

// LegcyModule represents a legacy module with InitManager method.
type LegcyModule interface {
	InitManager(mgr manager.Manager, env bootstrap.Environment, cbs InitCallbacks) error
}

type readyChecker struct {
	name    string
	checker func() error
}

type moduleReadyManager struct {
	mut                 sync.RWMutex
	moduleReadyCheckers map[string][]readyChecker
}

func (rm *moduleReadyManager) addReadyChecker(module, name string, checker func() error) {
	rm.mut.Lock()
	defer rm.mut.Unlock()

	dup := make(map[string][]readyChecker, len(rm.moduleReadyCheckers))
	for k, v := range rm.moduleReadyCheckers {
		dup[k] = v
	}

	dup[module] = append(dup[module], readyChecker{name, checker})
	rm.moduleReadyCheckers = dup
}

func (rm *moduleReadyManager) check() error {
	rm.mut.RLock()
	checkers := rm.moduleReadyCheckers
	rm.mut.RUnlock()

	var buf *bytes.Buffer
	for m, mCheckers := range checkers {
		for _, chk := range mCheckers {
			if err := chk.checker(); err != nil {
				if buf == nil {
					buf = &bytes.Buffer{}
					buf.WriteString(fmt.Sprintf("module %s checker %s not ready %v\n", m, chk.name, err))
				}
			}
		}
	}

	if buf == nil {
		return nil
	}
	return errors.New(buf.String())
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
		le       leaderelection.LeaderElector
		mgrOpts  ctrl.Options
	)
	for _, mc := range mcs {
		modKinds = append(modKinds, mc.module.Kind())
		if err := mc.module.InitScheme(scheme); err != nil {
			log.Errorf("mod %s InitScheme met err %v", mc.module.Kind(), err)
			fatal()
		}
	}

	var conf *restclient.Config
	if config.Global != nil && config.Global.GetMasterUrl() != "" {
		if conf, err = clientcmd.BuildConfigFromFlags(config.Global.GetMasterUrl(), ""); err != nil {
			log.Errorf("unable to build rest client by %s", config.Global.GetMasterUrl())
			os.Exit(1)
		}
	} else {
		conf = ctrl.GetConfigOrDie()
	}

	if config.Global != nil && config.Global.ClientGoTokenBucket != nil {
		conf.Burst = int(config.Global.ClientGoTokenBucket.Burst)
		conf.QPS = float32(config.Global.ClientGoTokenBucket.Qps)
		log.Infof("set burst: %d, qps %f based on user-specified value in client config", conf.Burst, conf.QPS)
	}

	// setup for leaderelection
	if config.Global.Misc["enable-leader-election"] == "on" {
		rl, err := leaderelection.NewKubeResourceLock(conf, "", bundle)
		if err != nil {
			log.Errorf("create kube reource lock failed: %v", err)
			fatal()
		}
		le = leaderelection.NewKubeLeaderElector(rl)
		mgrOpts = mgrOptionsWithLeaderElection(mgrOpts, rl)
	} else {
		le = leaderelection.NewAlwaysLeader()
	}

	mgrOpts.Scheme = scheme
	mgrOpts.MetricsBindAddress = config.Global.Misc["metrics-addr"]
	mgrOpts.Port = 9443
	mgr, err := ctrl.NewManager(conf, mgrOpts)
	if err != nil {
		log.Errorf("unable to create manager %s, %+v", bundle, err)
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

	// parse pathRedirect param
	pathRedirects := make(map[string]string)
	if config.Global.Misc["pathRedirect"] != "" {
		mappings := strings.Split(config.Global.Misc["pathRedirect"], ",")
		for _, m := range mappings {
			paths := strings.Split(m, "->")
			if len(paths) != 2 {
				log.Errorf("pathRedirect '%s' parse error: ilegal expression", m)
				continue
			}
			redirectPath, path := paths[0], paths[1]
			pathRedirects[redirectPath] = path
		}
	}

	ph := bootstrap.NewPathHandler(pathRedirects)

	readyMgr := &moduleReadyManager{moduleReadyCheckers: map[string][]readyChecker{}}

	// init configController if configSource field is used
	cc, err := bootstrap.NewConfigController(config, mgr.GetConfig())
	if err != nil {
		log.Errorf("start ConfigController error: %+v", err)
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
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
					rawMsg, ok := modSelfCfg.(*util.AnyMessage)
					if len(modCfg.General.XXX_unrecognized) > 0 {
						if ok {
							rawMsg.Raw = modCfg.General.XXX_unrecognized
						} else if err := proto.Unmarshal(modCfg.General.XXX_unrecognized, modSelfCfg); err != nil {
							log.Errorf("unmarshal for mod %s XXX_unrecognized (%v) met err %v", modCfg.Name, modCfg.General.XXX_unrecognized, err)
							fatal()
						}
					} else if len(modGeneralJson) > 0 {
						u := jsonpb.Unmarshaler{AllowUnknownFields: true}
						if ok {
							rawMsg.RawJson = modGeneralJson
						} else if err := u.Unmarshal(bytes.NewBuffer(modGeneralJson), modSelfCfg); err != nil {
							log.Errorf("unmarshal for mod %s modGeneralJson (%v) met err %v", modCfg.Name, modGeneralJson, err)
							fatal()
						}
					}
				}
			}

		}

		env := bootstrap.Environment{
			Config:           modCfg,
			ConfigController: cc,
			K8SClient:        clientSet,
			DynamicClient:    dynamicClient,
			ReadyManager: bootstrap.ReadyManagerFunc(func(moduleName string) func(name string, checker func() error) {
				return func(name string, checker func() error) {
					readyMgr.addReadyChecker(moduleName, name, checker)
				}
			}(modCfg.Name)),
			HttpPathHandler: bootstrap.PrefixPathHandlerManager{
				Prefix:      modCfg.Name,
				PathHandler: ph,
			},
			Stop: ctx.Done(),
		}
		if lm, ok := mc.module.(LegcyModule); ok {
			if err := lm.InitManager(mgr, env, cbs); err != nil {
				log.Errorf("mod %s InitManager met err %v", modCfg.Name, err)
				fatal()
			}
		} else {
			if err := mc.module.Init(env); err != nil {
				log.Errorf("mod %s Init met err %v", modCfg.Name, err)
				fatal()
			}
			if err := mc.module.SetupWithInitCallbacks(cbs); err != nil {
				log.Errorf("mod %s SetupWithInitCallbacks met err %v", modCfg.Name, err)
				fatal()
			}
			if err := mc.module.SetupWithManager(mgr); err != nil {
				log.Errorf("mod %s SetupWithManager met err %v", modCfg.Name, err)
				fatal()
			}

			if err := mc.module.SetupWithLeaderElection(le); err != nil {
				log.Errorf("mod %s SetupWithLeaderElection met err %v", modCfg.Name, err)
				fatal()
			}
		}
	}

	go func() {
		auxAddr := config.Global.Misc["aux-addr"]
		bootstrap.AuxiliaryHttpServerStart(ph, auxAddr, pathRedirects, readyMgr.check)
	}()

	for _, startup := range startups {
		startup(ctx)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Infof("starting bundle %s with modules %v", bundle, modKinds)
		if err := le.Run(ctx); err != nil {
			log.Errorf("problem running, %+v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Infof("starting manager with modules %v", modKinds)
		if err := mgr.Start(ctx); err != nil {
			log.Errorf("problem running, %+v", err)
		}
	}()

	wg.Wait()
}

// Merge The content of dst will not be changed, return a new instance with merged result
func merge(dst, src proto.Message) proto.Message {
	ret := proto.Clone(dst)
	proto.Merge(ret, src)
	return ret
}

// mgrOptionsWithLeaderElection uses reflect to set the manager's resourcelock
// instead of creating by it yourself. This way we keep the election state of
// ctrl manager and slime leader selector in sync.
func mgrOptionsWithLeaderElection(opts ctrl.Options, rl resourcelock.Interface) ctrl.Options {
	opts.LeaderElection = true
	f := func(_ *rest.Config, _ recorder.Provider, _ ctrlleaderelection.Options) (resourcelock.Interface, error) {
		return rl, nil
	}
	v := reflect.ValueOf(&opts).Elem()
	for i := 0; i < v.NumField(); i++ {
		if v.Type().Field(i).Name != "newResourceLock" {
			continue
		}
		vf := v.Field(i)
		vf = reflect.NewAt(vf.Type(), unsafe.Pointer(vf.UnsafeAddr())).Elem()
		vf.Set(reflect.ValueOf(f))
	}
	return opts
}
