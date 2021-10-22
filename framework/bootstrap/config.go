package bootstrap

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/gogo/protobuf/jsonpb"
	"k8s.io/client-go/kubernetes"
	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
)

const (
	DefaultModuleConfigPath = "/etc/slime/config/cfg"
)

var defaultModuleConfig = &bootconfig.Config{
	Enable:  false,
	Limiter: &bootconfig.Limiter{},
	Plugin:  &bootconfig.Plugin{},
	Fence:   &bootconfig.Fence{},
	Global: &bootconfig.Global{
		Service:        "app",
		IstioNamespace: "istio-system",
		SlimeNamespace: "mesh-operator",
		Log: &bootconfig.Log{
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

func patchModuleConfig(config, patch *bootconfig.Config) {
	if config.Global == nil {
		config.Global = patch.Global
	} else {
		patchGlobal(config.Global, patch.Global)
	}
	return
}

func patchGlobal(global, patch *bootconfig.Global) {
	if global.Service == "" {
		global.Service = patch.Service
	}
	if global.IstioNamespace == "" {
		global.IstioNamespace = patch.IstioNamespace
	}
	if global.SlimeNamespace == "" {
		global.SlimeNamespace = patch.SlimeNamespace
	}

	if len(global.Misc) == 0 {
		global.Misc = patch.Misc
	} else {
		for k, v := range patch.Misc {
			if _, ok := global.Misc[k]; !ok {
				global.Misc[k] = v
			}
		}
	}

	if global.Log == nil {
		global.Log = patch.Log
	} else {
		if global.Log.LogLevel == "" {
			global.Log.LogLevel = patch.Log.LogLevel
		}
		if global.Log.KlogLevel == 0 {
			global.Log.KlogLevel = patch.Log.KlogLevel
		}
	}
}

func GetModuleConfig(name string) (*bootconfig.Config, []byte, []byte, error) {
	filePath := DefaultModuleConfigPath
	if name != "" {
		filePath += "_" + name
	}
	cfg, raw, generalJson, err := readModuleConfig(filePath)
	if err != nil {
		return nil, nil, nil, err
	}

	patchModuleConfig(cfg, defaultModuleConfig)
	return cfg, raw, generalJson, nil
}

func readModuleConfig(filePath string) (*bootconfig.Config, []byte, []byte, error) {
	raw, err := ioutil.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return nil, nil, nil, err
	}

	// as jsonpb does not support XXX_unrecognized
	var m map[string]interface{}
	var generalJson []byte
	if err = json.Unmarshal(raw, &m); err != nil {
		return nil, nil, nil, err
	} else if m != nil {
		gen := m["general"]
		if gen != nil {
			if generalJson, err = json.Marshal(gen); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	c := &bootconfig.Config{}
	um := jsonpb.Unmarshaler{AllowUnknownFields: true}
	err = um.Unmarshal(bytes.NewBuffer(raw), c)
	if err != nil {
		return nil, nil, nil, err
	}
	return c, raw, generalJson, nil
}

type Environment struct {
	Config    *bootconfig.Config
	K8SClient *kubernetes.Clientset
	Stop      <-chan struct{}
}

func (env *Environment) IstioRev() string {
	if env == nil || env.Config == nil || env.Config.Global == nil {
		return ""
	}
	return env.Config.Global.IstioRev
}
