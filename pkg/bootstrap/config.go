package bootstrap

import (
	"io/ioutil"
	"os"

	"github.com/gogo/protobuf/jsonpb"
	"k8s.io/client-go/kubernetes"

	netease_config "yun.netease.com/slime/pkg/apis/config/v1alpha1"
)

const (
	ConfigPath        = "CONFIG_PATH"
	DefaultConfigPath = "/etc/slime/config/cfg"
)

var (
	defaultConfig = &netease_config.Config{
		Limiter: &netease_config.Limiter{
			Enable: false,
		},
		Plugin: &netease_config.Plugin{
			Enable: false,
		},
		Fence: &netease_config.Fence{
			Enable: false,
		},
		Global: &netease_config.Global{
			Service:        "app",
			IstioNamespace: "istio-system",
			SlimeNamespace: "mesh-operator",
		},
	}
)

func GetConfig() *netease_config.Config {
	var path string
	if p, found := os.LookupEnv(ConfigPath); !found {
		path = DefaultConfigPath
	} else {
		path = p
	}

	if config, err := read(path); err != nil {
		return defaultConfig
	} else {
		if config.Fence == nil {
			config.Fence = defaultConfig.Fence
		}
		if config.Limiter == nil {
			config.Limiter = defaultConfig.Limiter
		}
		if config.Plugin == nil {
			config.Plugin = defaultConfig.Plugin
		}
		if config.Global == nil {
			config.Global = defaultConfig.Global
		}
		return config
	}

}

func read(config string) (*netease_config.Config, error) {
	y, err := ioutil.ReadFile(config)
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
