package bootstrap

import (
	"io/ioutil"

	"github.com/gogo/protobuf/jsonpb"
	"k8s.io/client-go/kubernetes"
	netease_config "slime.io/slime/slime-framework/apis/config/v1alpha1"
)

const (
	DefaultModuleConfigPath = "/etc/slime/config/cfg"
)

var (
	defaultModuleConfig = &netease_config.Config{
		Enable: false,
		Limiter: &netease_config.Limiter{},
		Plugin: &netease_config.Plugin{},
		Fence: &netease_config.Fence{},
		Global: &netease_config.Global{
			Service:        "app",
			IstioNamespace: "istio-system",
			SlimeNamespace: "mesh-operator",
		},
	}
)

func GetModuleConfig() *netease_config.Config {
	if config, err := readModuleConfig(); err != nil {
		return defaultModuleConfig
	} else {
		if config.Fence == nil {
			config.Fence = defaultModuleConfig.Fence
		}
		if config.Limiter == nil {
			config.Limiter = defaultModuleConfig.Limiter
		}
		if config.Plugin == nil {
			config.Plugin = defaultModuleConfig.Plugin
		}
		if config.Global == nil {
			config.Global = defaultModuleConfig.Global
		}
		return config
	}
}

func readModuleConfig() (*netease_config.Config, error) {
	y, err := ioutil.ReadFile(DefaultModuleConfigPath)
	if err != nil {
		return nil, err
	}
	c := &netease_config.Config{}
	err = jsonpb.UnmarshalString(string(y), c)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type Environment struct {
	Config    *netease_config.Config
	K8SClient *kubernetes.Clientset
	Stop      <-chan struct{}
}
