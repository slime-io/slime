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

	"contrib.go.opencensus.io/exporter/prometheus"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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
	"slime.io/slime/framework/monitoring"
	"slime.io/slime/framework/util"
)

type InitCallbacks struct {
	AddStartup func(func(ctx context.Context))
}

type moduleConfig struct {
	module Module
	config *bootconfig.Config
}

type configH struct {
	config      *bootconfig.Config
	generalJson []byte
}

// ModuleOptions carries the framework context for setting a module.
// For fields marked with REQUIRED, the framework will ensure their existence,
// and the module can be used directly.
// NOTE: the Manager and the LeaderElectionCbs share the election status.
// When switching from leader to candidate, the Manager will exit.
// The framework will close the LeaderElection and ends the process after
// the Manager exits. Therefore, when implementing a module, the scenario of
// becoming the leader again may not be considered.
type ModuleOptions struct {
	// Env is the common environment context used by the module.
	// REQUIRED
	Env bootstrap.Environment

	// InitCbs is used to register callback functions that support concurrent
	// execution. The callback must be non-blocking and can get state via ctx.
	// REQUIRED
	InitCbs InitCallbacks

	// Manager is used to manager controller.
	// Registers `Reconciler` with Manager to build a controller.
	// REQUIRED
	Manager manager.Manager

	// LeaderElectionCbs is used to registers callbacks that require
	// a single instance to run.
	// Generally, resident services that run concurrently and trigger the
	// creation and update of resources in the cluster may involve race
	// conditions and cause system exceptions. The startup of these services
	// must be controlled through an election mechanism.
	// Currently, only the following state transfers are supported:
	//   1. START -> candidate -> leader -> EXIT
	//   2. START -> candidate -> EXIT
	// REQUIRED
	LeaderElectionCbs leaderelection.LeaderCallbacks
}

type Module interface {
	// Setup we will only register run-able functions in Setup and no actual execution will take place in Setup
	// the module user should not add blocking run-able functions in Setup
	Setup(opts ModuleOptions) error
	Kind() string
	Config() proto.Message
	InitScheme(scheme *runtime.Scheme) error
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

func LoadModule(name string, modGetter func(modCfg *bootconfig.Config) Module, bundleConfig *bootconfig.Config) (Module, *bootstrap.ParsedModuleConfig, error) {
	pmCfg, err := bootstrap.GetModuleConfig(name)
	if err != nil {
		return nil, nil, err
	}

	mod, err := LoadModuleFromConfig(pmCfg, modGetter, bundleConfig)
	if err != nil {
		return nil, nil, err
	}

	return mod, pmCfg, nil
}

func LoadModuleFromConfig(pmCfg *bootstrap.ParsedModuleConfig, modGetter func(modCfg *bootconfig.Config) Module, bundleConfig *bootconfig.Config) (Module, error) {
	mod := modGetter(pmCfg.Config)
	modCfg := pmCfg.Config
	if modCfg.Bundle != nil || mod == nil {
		return nil, nil
	}

	// not bundle

	mod = mod.Clone()

	if bundleConfig != nil {
		if bundleConfig.Global != nil {
			modCfg.Global = merge(bundleConfig.Global, modCfg.Global).(*bootconfig.Global)
		}
	}

	modConfigJson, err := json.Marshal(*modCfg)
	if err != nil {
		return nil, err
	}
	pmCfg.RawJson = modConfigJson

	modSelfCfg := mod.Config()
	if modSelfCfg == nil {
		return mod, nil
	}

	// get mod.Config() value from config.general
	if len(pmCfg.GeneralJson) > 0 {
		if err := protojson.Unmarshal(pmCfg.GeneralJson, modSelfCfg); err != nil {
			log.Errorf("unmarshal for mod %s modGeneralJson (%v) met err %v", modCfg.Name, pmCfg.GeneralJson, err)
			fatal()
		}
	}

	return mod, nil
}

func fatal() {
	os.Exit(1)
}

func Main(bundle string, modules []Module) {
	// prepare module definition map
	moduleDefinitions := make(map[string]Module)
	for _, mod := range modules {
		moduleDefinitions[mod.Kind()] = mod
	}

	// Init module of instance
	var mcs []*moduleConfig

	modGetter := func(pmCfg *bootconfig.Config) Module {
		var m Module
		if pmCfg.Kind != "" {
			m = moduleDefinitions[pmCfg.Kind]
		} else {
			// compatible for old version without kind field
			m = moduleDefinitions[pmCfg.Name]
		}
		return m
	}
	// get main module config
	mainMod, mainModParsedCfg, err := LoadModule("", modGetter, nil)
	if err != nil {
		panic(err)
	}
	mainModConfig, mainModRawJson, mainModGeneralJson := mainModParsedCfg.Config,
		mainModParsedCfg.RawJson, mainModParsedCfg.GeneralJson
	if mainModConfig == nil {
		panic(fmt.Errorf("module config nil for %s", bundle))
	}
	err = util.InitLog(mainModConfig.Global.Log)
	if err != nil {
		panic(err)
	}

	log.Infof("load module config of %s: %s, generalCfg: %s", bundle, string(mainModRawJson), string(mainModGeneralJson))

	// check if main module is bundle or not
	isBundle := mainModConfig.Bundle != nil
	if !isBundle {
		if mainMod == nil {
			log.Errorf("mod nil for %s", mainModConfig.Name)
			fatal()
		}

		mc := &moduleConfig{
			module: mainMod,
			config: mainModConfig,
		}

		if mainModConfig.Enable {
			mcs = append(mcs, mc)
		}
	} else {
		for _, modCfg := range mainModConfig.Bundle.Modules {
			mod, modParsedCfg, err := LoadModule(modCfg.Name, modGetter, mainModConfig)
			if err != nil {
				panic(err)
			}
			if mod == nil {
				log.Errorf("mod nil for %s", modCfg.Name)
				fatal()
			}

			log.Infof("load raw module config of bundle item %s: %s, general: %s",
				modCfg.Name, string(modParsedCfg.RawJson), string(modParsedCfg.GeneralJson))

			mc := &moduleConfig{
				module: mod,
				config: modParsedCfg.Config,
			}

			if mc.config.Enable {
				mcs = append(mcs, mc)
			}
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
	if mainModConfig.Global != nil && mainModConfig.Global.GetMasterUrl() != "" {
		if conf, err = clientcmd.BuildConfigFromFlags(mainModConfig.Global.GetMasterUrl(), ""); err != nil {
			log.Errorf("unable to build rest client by %s", mainModConfig.Global.GetMasterUrl())
			os.Exit(1)
		}
	} else {
		conf = ctrl.GetConfigOrDie()
	}

	if mainModConfig.Global != nil && mainModConfig.Global.ClientGoTokenBucket != nil {
		conf.Burst = int(mainModConfig.Global.ClientGoTokenBucket.Burst)
		conf.QPS = float32(mainModConfig.Global.ClientGoTokenBucket.Qps)
		log.Infof("set burst: %d, qps %f based on user-specified value in client config", conf.Burst, conf.QPS)
	}

	// setup for leaderelection
	if mainModConfig.Global.Misc["enableLeaderElection"] == "on" {

		deployRev := mainModConfig.Global.GetDeployRev()
		if deployRev != "" {
			bundle = fmt.Sprintf("%s-%s", bundle, deployRev)
		}

		// create a resource lock in the same namespace as the workload instance
		rl, err := leaderelection.NewKubeResourceLock(conf, os.Getenv("WATCH_NAMESPACE"), bundle)
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
	mgrOpts.MetricsBindAddress = mainModConfig.Global.Misc["metrics-addr"]
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
	if mainModConfig.Global.Misc["pathRedirect"] != "" {
		mappings := strings.Split(mainModConfig.Global.Misc["pathRedirect"], ",")
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

	var once sync.Once
	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())

	// init ConfigController
	var configController, istioConfigController bootstrap.ConfigController

	if mainModConfig.GetGlobal() != nil && len(mainModConfig.GetGlobal().GetConfigSources()) > 0 {
		configController, err = bootstrap.NewConfigController(mainModConfig.GetGlobal().GetConfigSources(), ctx.Done())
		if err != nil {
			log.Warnf("new ConfigController failed: %+v", err)
			configController = nil
		}
	}

	if mainModConfig.GetGlobal() != nil && mainModConfig.GetGlobal().IstioConfigSource != nil {
		istioConfigController, err = bootstrap.NewConfigController([]*bootconfig.ConfigSource{mainModConfig.GetGlobal().IstioConfigSource}, ctx.Done())
		if err != nil {
			log.Warnf("new IstioConfigController error: %+v", err)
			istioConfigController = nil
		}
	}

	env := bootstrap.Environment{
		ConfigController:      configController,
		IstioConfigController: istioConfigController,
		K8SClient:             clientSet,
		DynamicClient:         dynamicClient,
		HttpPathHandler:       ph,
		Stop:                  ctx.Done(),
	}

	// setup modules
	monitoring.SubModulesCount.Record(float64(len(mcs)))
	for _, mc := range mcs {
		modCfg := mc.config
		moduleEnv := bootstrap.Environment{
			Config:                modCfg,
			ConfigController:      configController,
			IstioConfigController: istioConfigController,
			K8SClient:             clientSet,
			DynamicClient:         dynamicClient,
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
			if err := lm.InitManager(mgr, moduleEnv, cbs); err != nil {
				log.Errorf("mod %s InitManager met err %v", modCfg.Name, err)
				fatal()
			}
		} else {
			if err := mc.module.Setup(ModuleOptions{
				Env:               moduleEnv,
				InitCbs:           cbs,
				Manager:           mgr,
				LeaderElectionCbs: le,
			}); err != nil {
				log.Errorf("mod %s Setup met err %v", modCfg.Name, err)
				fatal()
			}
		}
	}

	// run ConfigController
	if configController != nil {
		configController, err = bootstrap.RunController(configController, mainModConfig, mgr.GetConfig())
		if err != nil {
			log.Errorf("run config controller failed: %s", err)
			return
		}
	}

	if istioConfigController != nil {
		istioConfigController, err = bootstrap.RunIstioController(istioConfigController, mainModConfig)
		if err != nil {
			log.Errorf("run config controller failed: %s", err)
			return
		}
	}

	// Create the Prometheus exporter.
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: util.MetricPrefix,
	})
	if err != nil {
		log.Fatalf("Failed to create the Prometheus stats exporter: %v", err)
	}

	go func() {
		auxAddr := mainModConfig.Global.Misc["aux-addr"]
		bootstrap.AuxiliaryHttpServerStart(env, ph, auxAddr, pathRedirects, readyMgr.check, pe)
	}()

	// Run the runnable function registered by the submodule
	for _, startup := range startups {
		startup(ctx)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer once.Do(cancel)
		log.Infof("starting bundle %s with modules %v", bundle, modKinds)
		if err := le.Run(ctx); err != nil {
			log.Errorf("problem running, %+v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer once.Do(cancel)
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
	f := func(_ *restclient.Config, _ recorder.Provider, _ ctrlleaderelection.Options) (resourcelock.Interface, error) {
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
