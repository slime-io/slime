package bootstrap

import (
	"encoding/json"
	"os"
	"strings"

	ghYaml "github.com/ghodss/yaml"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kube-openapi/pkg/common"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
)

const (
	DefaultModuleConfigPath = "/etc/slime/config/cfg"
)

var defaultModuleConfig = &bootconfig.Config{
	Enable: false,
	Global: &bootconfig.Global{
		Service:        "app",
		IstioNamespace: "istio-system",
		SlimeNamespace: "mesh-operator",
		Log: &bootconfig.Log{
			LogLevel:  "",
			KlogLevel: 0,
			LogRotate: false,
			LogRotateConfig: &bootconfig.LogRotateConfig{
				FilePath:   "/tmp/log/slime.log",
				MaxSizeMB:  100,
				MaxBackups: 10,
				MaxAgeDay:  10,
				Compress:   false,
			},
		},
		Misc: map[string]string{
			"metrics-addr":         ":8080",
			"aux-addr":             ":8081",
			"enableLeaderElection": "off",
			"logSourcePort":        ":8082",
			// which label keys of serviceEntry select endpoints
			// will take effect when serviceEntry does not have workloadSelector field
			"seLabelSelectorKeys": "app",
			// indicate whether xds config source enable increment push or not
			"xdsSourceEnableIncPush": "true",
			// path redirect mapping of aux server, default is null
			"pathRedirect": "",
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
		if global.Log.LogRotate {
			if global.Log.LogRotateConfig == nil {
				global.Log.LogRotateConfig = patch.Log.LogRotateConfig
			} else {
				if global.Log.LogRotateConfig.FilePath == "" {
					global.Log.LogRotateConfig.FilePath = patch.Log.LogRotateConfig.FilePath
				}
				if global.Log.LogRotateConfig.MaxSizeMB == 0 {
					global.Log.LogRotateConfig.MaxSizeMB = patch.Log.LogRotateConfig.MaxSizeMB
				}
				if global.Log.LogRotateConfig.MaxBackups == 0 {
					global.Log.LogRotateConfig.MaxBackups = patch.Log.LogRotateConfig.MaxBackups
				}
				if global.Log.LogRotateConfig.MaxAgeDay == 0 {
					global.Log.LogRotateConfig.MaxAgeDay = patch.Log.LogRotateConfig.MaxAgeDay
				}
			}
		}
	}
}

type ParsedModuleConfig struct {
	Config      *bootconfig.Config
	RawJson     []byte
	GeneralJson []byte
}

func GetModuleConfig(name string) (*ParsedModuleConfig, error) {
	filePath := DefaultModuleConfigPath
	if name != "" {
		filePath += "_" + name
	}

	cfgData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return nil, err
	}

	return LoadModuleConfigFromData(cfgData, filePath == DefaultModuleConfigPath)
}

func LoadModuleConfigFromData(data []byte, patchDefault bool) (*ParsedModuleConfig, error) {
	parsed, err := parseModuleConfig(data)
	if err != nil {
		return nil, err
	}
	if parsed.Config == nil {
		parsed.Config = &bootconfig.Config{}
	}

	if patchDefault {
		patchModuleConfig(parsed.Config, defaultModuleConfig)
	}
	return parsed, nil
}

func parseModuleConfig(data []byte) (*ParsedModuleConfig, error) {
	var (
		c                    = &bootconfig.Config{}
		err                  error
		rawJson, generalJson []byte
		m                    map[string]interface{}
	)

	// convert yaml/json to json
	rawJson, err = ghYaml.YAMLToJSON(data)
	if err != nil {
		return nil, err
	}

	// as jsonpb does not support XXX_unrecognized
	if err = json.Unmarshal(rawJson, &m); err != nil {
		return nil, err
	} else if m != nil {
		gen := m["general"]
		if gen != nil {
			if generalJson, err = json.Marshal(gen); err != nil {
				return nil, err
			}
		}
	}

	err = protojson.Unmarshal(rawJson, c)
	if err != nil {
		return nil, err
	}

	return &ParsedModuleConfig{
		Config:      c,
		RawJson:     rawJson,
		GeneralJson: generalJson,
	}, nil
}

type ReadyManager interface {
	AddReadyChecker(name string, checker func() error)
}

type ReadyManagerFunc func(name string, checker func() error)

func (f ReadyManagerFunc) AddReadyChecker(name string, checker func() error) {
	f(name, checker)
}

type Environment struct {
	Config *bootconfig.Config

	// clientSet, not support crd, it can use in anytime anywhere
	K8SClient *kubernetes.Clientset
	// dynamic client, support any resource
	DynamicClient    dynamic.Interface
	HttpPathHandler  common.PathHandler
	ReadyManager     ReadyManager
	Stop             <-chan struct{}
	ConfigController ConfigController
	// a mirror of relevant istiod internal configs
	IstioConfigController ConfigController
}

func (env *Environment) IstioRev() string {
	if env == nil || env.Config == nil || env.Config.Global == nil {
		return ""
	}
	return env.Config.Global.IstioRev
}

// RevInScope check revision
// when StrictRev is true, return true if revision in global.IstioRev
// when StrictRev is false, return true if revision in global.IstioRev or revision is empty or global.IstioRev is empty
func (env *Environment) RevInScope(rev string) bool {
	// if IstioRev is "", strictRev is meaningless, we will manage all resource
	if env == nil || env.Config == nil || env.Config.Global == nil || env.Config.Global.IstioRev == "" {
		return true
	}

	istioRevs := strings.Split(env.Config.Global.IstioRev, ",")

	if env.Config.Global.StrictRev {
		return inRevs(rev, istioRevs)
	} else {
		return rev == "" || inRevs(rev, istioRevs)
	}
}

// SelfResourceRev
// if SelfResourceRev is specified, the value of SelfResourceRev will be patched to resources which generated by slime itself, just like serviceFence
// smartlimiter/envoyplugin/pluginmanager is excluded because these envoyfilter are generated by users or higher-level services
// envoyfilter/sidecar is excluded because these resources generated base on their owner resources

func (env *Environment) SelfResourceRev() string {
	if env == nil || env.Config == nil || env.Config.Global == nil {
		return ""
	}
	return env.Config.Global.SelfResourceRev
}

func inRevs(rev string, revs []string) bool {
	for _, item := range revs {
		item = strings.Trim(item, " ")

		if item == rev {
			return true
		}
	}
	return false
}
