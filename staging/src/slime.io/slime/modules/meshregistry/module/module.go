package module

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mitchellh/copystructure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/module"
	meshregv1alpha1 "slime.io/slime/modules/meshregistry/api/v1alpha1"
	"slime.io/slime/modules/meshregistry/model"
	meshregbootstrap "slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/server"
)

const (
	EnvPodNamespace = "POD_NAMESPACE"
)

var log = model.ModuleLog

var PodNamespace = os.Getenv(EnvPodNamespace) // TODO passed by framework

type structWrapper struct {
	*structpb.Struct
}

func (s *structWrapper) Value() []byte {
	value, _ := protojson.Marshal(s)
	return value
}

type Module struct {
	config                  structWrapper
	reloadDynamicConfigTask func(ctx context.Context)
	dynConfigHandlers       []func(args *meshregbootstrap.RegistryArgs)
	mut                     sync.RWMutex
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
		networkingv1alpha3.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) Clone() module.Module {
	ret := Module{}
	if m.config.Struct != nil {
		ret.config.Struct = proto.Clone(m.config.Struct).(*structpb.Struct)
	} else {
		ret.config.Struct = &structpb.Struct{}
	}
	return &ret
}

func ParseArgsFromModuleConfig(config *structWrapper) (*meshregbootstrap.RegistryArgs, error) {
	regArgs := meshregbootstrap.NewRegistryArgs()

	type legacyWrapper struct {
		Legacy json.RawMessage `json:"LEGACY"`
	}

	if rawJson := config.Value(); len(rawJson) > 0 {
		var lw legacyWrapper
		if err := json.Unmarshal(rawJson, &lw); err != nil {
			log.Errorf("invalid raw json: %s", string(rawJson))
			return nil, err
		}

		if lw.Legacy != nil {
			if err := json.Unmarshal(lw.Legacy, regArgs); err != nil {
				log.Errorf("invalid raw json: %s", string(rawJson))
				return nil, err
			}
		}
	}

	return regArgs, nil
}

type singleConfigMapCache struct {
	cm  *corev1.ConfigMap
	mut sync.RWMutex
}

func (c *singleConfigMapCache) Get() *corev1.ConfigMap {
	c.mut.RLock()
	defer c.mut.RUnlock()

	return c.cm
}

func (c *singleConfigMapCache) Set(cm *corev1.ConfigMap) *corev1.ConfigMap {
	c.mut.Lock()
	defer c.mut.Unlock()

	c.cm, cm = cm, c.cm
	return cm
}

func configMapModuleKey(name string) string {
	return "cfg_" + name
}

func (m *Module) prepareDynamicConfigController(opts module.ModuleOptions, staticRegArgs *meshregbootstrap.RegistryArgs) (*meshregbootstrap.RegistryArgs, error) {
	// TODO:
	// We define and use two types of configuration Configurator for hot updates:
	//   - FullConfigurator for replace behavior
	//   - PatchConfigurator for patch behavior
	// Currently, instead of providing a common framework for registering and executing these configuration events,
	// We directly 'hardcode' in the `prepareDynamicConfigController` method. It is expected that a universal
	// configuration hot update framework will be implemented in the Framework module to replace the internal implementation
	// of the meshregistry module and can be used by other module.

	// 1. init full configuration Configurator
	//    - load dynamic config from ConfigMap specified by env var DYNAMIC_CONFIG_MAP
	//    - or return original static config
	// 2. init patch configuration Configurator
	// 	  - watching RegistrySource CR specified by env var WATCHING_REGISTRYSOURCE
	dynConfigMapName, dynRegistrySource := os.Getenv("DYNAMIC_CONFIG_MAP"), os.Getenv("WATCHING_REGISTRYSOURCE")
	if dynConfigMapName == "" && dynRegistrySource == "" {
		return nil, nil
	}

	changeNotifyCh := make(chan struct{}, 1)

	var (
		fullConfigurator  func() (*meshregbootstrap.RegistryArgs, error)
		patchConfigurator func(*meshregbootstrap.RegistryArgs) (*meshregbootstrap.RegistryArgs, error)
		wait              func() bool
	)

	if dynConfigMapName != "" {
		fullConfigurator, wait = m.prepareCmDynamicConfigController(dynConfigMapName, changeNotifyCh, opts, staticRegArgs)
		if !wait() {
			return nil, fmt.Errorf("failed to wait for configmap cache sync")
		}

	} else {
		fullConfigurator = func() (*meshregbootstrap.RegistryArgs, error) {
			cp, err := copystructure.Copy(staticRegArgs)
			return cp.(*meshregbootstrap.RegistryArgs), err
		}
	}

	if dynRegistrySource != "" {
		patchConfigurator, wait = m.prepareCrDynamicConfigController(dynRegistrySource, changeNotifyCh, opts)
		if !wait() {
			return nil, fmt.Errorf("failed to wait for configmap cache sync")
		}
	}

	reloadArg := func() (*meshregbootstrap.RegistryArgs, error) {
		args, err := fullConfigurator()
		if err != nil {
			return nil, err
		}
		if patchConfigurator != nil {
			args, err = patchConfigurator(args)
			if err != nil {
				return nil, err
			}
		}
		return args, nil
	}

	m.reloadDynamicConfigTask = func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-changeNotifyCh:
				log.Infof("reloadDynamicConfigTask get notified")
				regArgs, err := reloadArg()
				if err != nil {
					log.Errorf("load arg from cache met err %v, skip...", err)
					continue
				}

				m.mut.RLock()
				handlers := m.dynConfigHandlers
				m.mut.RUnlock()

				for _, h := range handlers {
					h(regArgs)
				}
			}
		}
	}

	select {
	case <-changeNotifyCh:
		log.Infof("cm notify: will get dynamic config and replace the original one")
		return reloadArg()
	default:
	}

	return nil, nil
}

func (m *Module) prepareCmDynamicConfigController(
	name string,
	changeNotifyCh chan struct{},
	opts module.ModuleOptions,
	staticRegArgs *meshregbootstrap.RegistryArgs,
) (func() (*meshregbootstrap.RegistryArgs, error), func() bool) {
	client := opts.Env.K8SClient
	ctx := context.Background() // TODO

	listOptions := metav1.ListOptions{FieldSelector: fields.Set{"metadata.name": name}.AsSelector().String()}
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().ConfigMaps(PodNamespace).List(ctx, listOptions)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().ConfigMaps(PodNamespace).Watch(ctx, listOptions)
		},
	}

	cmCache := &singleConfigMapCache{}

	loadArgFromCache := func() (*meshregbootstrap.RegistryArgs, error) {
		cm := cmCache.Get()
		if cm == nil {
			cp, err := copystructure.Copy(staticRegArgs)
			return cp.(*meshregbootstrap.RegistryArgs), err
		}

		var cmValue []byte
		itemKey := configMapModuleKey(opts.Env.Config.Name)
		value, ok := cm.Data[itemKey]
		if ok {
			cmValue = []byte(value)
		} else {
			compressedValue, ok := cm.BinaryData[itemKey]
			if !ok {
				return nil, nil
			}
			// support gzip compressed configuration
			buf := bytes.NewBuffer(compressedValue)
			gr, err := gzip.NewReader(buf)
			if err != nil {
				return nil, err
			}
			defer gr.Close()
			cmValue, err = io.ReadAll(gr)
			if err != nil {
				return nil, err
			}
		}

		cfgMsg, err := parseModuleConfig(cmValue)
		if err != nil {
			return nil, err
		}
		if cfgMsg == nil {
			return nil, nil
		}

		return ParseArgsFromModuleConfig(cfgMsg)
	}

	notify := func(obj, newObj interface{}) {
		var cm *corev1.ConfigMap
		if newObj != nil {
			newCm, ok := newObj.(*corev1.ConfigMap)
			if !ok {
				log.Errorf("not configmap")
				return
			}

			cm = newCm
		}
		_ = cmCache.Set(cm)

		select {
		case changeNotifyCh <- struct{}{}:
		default:
		}
	}

	_, controller := cache.NewInformer(lw, &corev1.ConfigMap{}, 60*time.Second, cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { notify(nil, obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { notify(oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { notify(obj, nil) },
	})
	go controller.Run(ctx.Done())

	return loadArgFromCache, func() bool {
		return cache.WaitForCacheSync(ctx.Done(), controller.HasSynced)
	}
}

func (m *Module) prepareCrDynamicConfigController(name string, changeNotifyCh chan struct{}, opts module.ModuleOptions) (func(*meshregbootstrap.RegistryArgs) (*meshregbootstrap.RegistryArgs, error), func() bool) {
	client := opts.Env.DynamicClient
	ctx := context.Background() // TODO

	listOptions := metav1.ListOptions{FieldSelector: fields.Set{"metadata.name": name}.AsSelector().String()}
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.Resource(meshregv1alpha1.RegistrySourcesResource).Namespace(PodNamespace).List(ctx, listOptions)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.Resource(meshregv1alpha1.RegistrySourcesResource).Namespace(PodNamespace).Watch(ctx, listOptions)
		},
	}

	notify := func(obj, newObj interface{}) {
		select {
		case changeNotifyCh <- struct{}{}:
		default:
		}
	}

	store, controller := cache.NewInformer(lw, &unstructured.Unstructured{}, 60*time.Second, cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { notify(nil, obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { notify(oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { notify(obj, nil) },
	})
	go controller.Run(ctx.Done())

	patcher := func(src *meshregbootstrap.RegistryArgs) (*meshregbootstrap.RegistryArgs, error) {
		got, exist, err := store.GetByKey(PodNamespace + "/" + name)
		if err != nil {
			return nil, err
		}
		if !exist {
			return src, nil
		}
		u, ok := got.(*unstructured.Unstructured)
		if !ok {
			return nil, fmt.Errorf("not unstructured")
		}

		var rs meshregv1alpha1.RegistrySource
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), &rs); err != nil {
			return nil, err
		}

		var patch meshregbootstrap.RegistryArgs
		meshregv1alpha1.ConvertRegistrySourceToArgs(&rs, &patch)
		return patchRegistryArgs(src, &patch)
	}

	return patcher, func() bool {
		return cache.WaitForCacheSync(ctx.Done(), controller.HasSynced)
	}
}

func patchRegistryArgs(src, patch *meshregbootstrap.RegistryArgs) (*meshregbootstrap.RegistryArgs, error) {
	if patch == nil {
		return src, nil
	}
	meshregbootstrap.Patch(src, patch)
	return src, nil
}

func parseModuleConfig(data []byte) (*structWrapper, error) {
	pmCfg, err := bootstrap.LoadModuleConfigFromData(data, false)
	if err != nil {
		return nil, err
	}

	mod, err := module.LoadModuleFromConfig(pmCfg, func(modCfg *v1alpha1.Config) module.Module {
		return &Module{}
	}, nil)
	if err != nil {
		return nil, err
	}

	return mod.Config().(*structWrapper), nil
}

func (m *Module) Setup(opts module.ModuleOptions) error {
	regArgs, err := ParseArgsFromModuleConfig(&m.config)
	if err != nil {
		return err
	}
	if regArgs == nil {
		return fmt.Errorf("nil registry args")
	}

	dynRegArgs, err := m.prepareDynamicConfigController(opts, regArgs)
	if err != nil {
		return err
	}

	if dynRegArgs != nil {
		bs, err := json.MarshalIndent(regArgs, "", "  ")
		log.Infof("init registry args but override by dynamic reg args: %s, err %v", string(bs), err)
		regArgs = dynRegArgs
	}

	if err := regArgs.Validate(); err != nil {
		return fmt.Errorf("invalid args for meshregsitry: %w", err)
	}

	bs, err := json.MarshalIndent(regArgs, "", "  ")
	log.Infof("inuse registry args: %s, err %v", string(bs), err)

	cbs := opts.InitCbs
	cbs.AddStartup(func(ctx context.Context) {
		if m.reloadDynamicConfigTask != nil {
			go m.reloadDynamicConfigTask(ctx)
		}

		go func() {
			// Create the server for the discovery service.
			registryServer, err := server.NewServer(&server.Args{
				SlimeEnv:     opts.Env,
				RegistryArgs: regArgs,
				AddOnRegArgs: func(onConfig func(args *meshregbootstrap.RegistryArgs)) {
					m.mut.Lock()
					m.dynConfigHandlers = append(m.dynConfigHandlers, onConfig)
					m.mut.Unlock()

					log.Infof("add new dyn config handler")
				},
			})
			if err != nil {
				log.Errorf("failed to create discovery service: %v", err)
				return
			}

			// Start the server
			if err := registryServer.Run(ctx.Done()); err != nil {
				log.Errorf("failed to start discovery service: %v", err)
				return
			}
		}()
	})

	return nil
}
