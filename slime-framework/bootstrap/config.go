package bootstrap

import (
	log "github.com/sirupsen/logrus"
	"io/ioutil"

	"github.com/gogo/protobuf/jsonpb"
	"k8s.io/client-go/kubernetes"
	netease_config "slime.io/slime/slime-framework/apis/config/v1alpha1"
)

const (
	DefaultModuleConfigPath = "/etc/slime/config/cfg"
)

var defaultModuleConfig = &netease_config.Config{
	Enable:  false,
	Limiter: &netease_config.Limiter{},
	Plugin:  &netease_config.Plugin{},
	Fence:   &netease_config.Fence{},
	Global: &netease_config.Global{
		Service:        "app",
		IstioNamespace: "istio-system",
		SlimeNamespace: "mesh-operator",
		Log: &netease_config.Log{
			LogLevel:  "",
			KlogLevel: 0,
		},
		Misc: map[string]string{
			"metrics-addr":           ":8080",
			"aux-addr":               ":8081",
			"enable-leader-election": "off",
			"global-sidecar-mode":    "namespace",
		},
	},
}

func GetModuleConfig() *netease_config.Config {
	if config, err := readModuleConfig(); err != nil {
		log.Errorf("readModuleConfig error: %v", err)
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
			return config
		}
		if config.Global.Service == "" {
			config.Global.Service = defaultModuleConfig.Global.Service
		}
		if config.Global.IstioNamespace == "" {
			config.Global.IstioNamespace = defaultModuleConfig.Global.IstioNamespace
		}
		if config.Global.SlimeNamespace == "" {
			config.Global.SlimeNamespace = defaultModuleConfig.Global.SlimeNamespace
		}
		if len(config.Global.Misc) == 0 {
			config.Global.Misc = defaultModuleConfig.Global.Misc
		} else {
			for k, v := range defaultModuleConfig.Global.Misc {
				if _, ok := config.Global.Misc[k]; !ok {
					config.Global.Misc[k] = v
				}
			}
		}
		if config.Global.Log == nil {
			config.Global.Log = defaultModuleConfig.Global.Log
			return config
		}
		if config.Global.Log.LogLevel == "" {
			config.Global.Log.LogLevel = defaultModuleConfig.Global.Log.LogLevel
		}
		if config.Global.Log.KlogLevel == 0 {
			config.Global.Log.KlogLevel = defaultModuleConfig.Global.Log.KlogLevel
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
