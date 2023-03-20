package module

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/framework/apis/config/v1alpha1"
	istionetworkingapi "slime.io/slime/framework/apis/networking/v1alpha3"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/framework/util"
	"slime.io/slime/modules/meshregistry/model"
	meshregbootstrap "slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/server"
)

const (
	EnvPodNamespace = "POD_NAMESPACE"
)

var log = model.ModuleLog

var (
	PodNamespace = os.Getenv(EnvPodNamespace) // TODO passed by framework
)

type Module struct {
	config                  util.AnyMessage
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
		istionetworkingapi.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) Clone() module.Module {
	ret := Module{}
	return &ret
}

func ParseArgsFromModuleConfig(config util.AnyMessage) (*meshregbootstrap.RegistryArgs, error) {
	regArgs := meshregbootstrap.NewRegistryArgs()

	type legacyWrapper struct {
		Legacy json.RawMessage `json:"LEGACY"`
	}

	if rawJson := config.RawJson; rawJson != nil {
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

func (m *Module) prepareDynamicConfigController(opts module.ModuleOptions) (*meshregbootstrap.RegistryArgs, error) {
	dynConfigMapName := os.Getenv("DYNAMIC_CONFIG_MAP")
	if dynConfigMapName == "" {
		return nil, nil
	}

	client := opts.Env.K8SClient
	ctx := context.Background() // TODO

	listOptions := metav1.ListOptions{FieldSelector: fields.Set{"metadata.name": dynConfigMapName}.AsSelector().String()}
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().ConfigMaps(PodNamespace).List(ctx, listOptions)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().ConfigMaps(PodNamespace).Watch(ctx, listOptions)
		},
	}

	changeNotifyCh := make(chan struct{}, 1)
	cmCache := &singleConfigMapCache{}

	loadArgFromCache := func() (*meshregbootstrap.RegistryArgs, error) {
		cm := cmCache.Get()
		if cm == nil {
			return nil, nil
		}
		cmValue, ok := cm.Data[configMapModuleKey(opts.Env.Config.Name)]
		if !ok {
			return nil, nil
		}

		anyMsg, err := parseModuleConfig([]byte(cmValue))
		if err != nil {
			return nil, err
		}
		if anyMsg == nil {
			return nil, nil
		}

		return ParseArgsFromModuleConfig(*anyMsg)
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

	if !cache.WaitForCacheSync(ctx.Done(), controller.HasSynced) {
		return nil, fmt.Errorf("failed to wait for configmap cache sync")
	}

	m.reloadDynamicConfigTask = func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-changeNotifyCh:
				log.Infof("reloadDynamicConfigTask get notified")
				regArgs, err := loadArgFromCache()
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
		return loadArgFromCache()
	default:
	}

	return nil, nil
}

func parseModuleConfig(data []byte) (*util.AnyMessage, error) {
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

	return mod.Config().(*util.AnyMessage), nil
}

func (m *Module) Setup(opts module.ModuleOptions) error {
	regArgs, err := ParseArgsFromModuleConfig(m.config)
	if err != nil {
		return err
	}
	if regArgs == nil {
		return fmt.Errorf("nil registry args")
	}

	dynRegArgs, err := m.prepareDynamicConfigController(opts)
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
