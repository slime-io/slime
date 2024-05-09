package resource

import (
	"errors"
	"strings"

	"istio.io/libistio/pkg/config"
	"istio.io/libistio/pkg/config/schema/gvk"
)

type (
	GroupVersionKind = config.GroupVersionKind
	Config           = config.Config
	Meta             = config.Meta
)

var (
	ServiceEntry = gvk.ServiceEntry
	Service      = gvk.Service
	Endpoints    = gvk.Endpoints
	Pod          = gvk.Pod
	ConfigMap    = gvk.ConfigMap

	IstioService  = GroupVersionKind{Group: "networking.istio.io", Version: "v1alpha3", Kind: "IstioService"}
	IstioEndpoint = GroupVersionKind{Group: "networking.istio.io", Version: "v1alpha3", Kind: "IstioEndpoint"}
)

var (
	EmptyGroupVersionKind = GroupVersionKind{}
	AllGroupVersionKind   = EmptyGroupVersionKind
)

func ParseGroupVersionKind(s string) (GroupVersionKind, error) {
	if s == "" {
		return EmptyGroupVersionKind, nil
	}

	parts := strings.SplitN(s, "/", 3)
	if len(parts) < 2 {
		return GroupVersionKind{}, errors.New("invalid value")
	}

	var ret GroupVersionKind

	switch len(parts) {
	case 3:
		ret.Group = parts[0]
		ret.Version = parts[1]
		ret.Kind = parts[2]
	case 2:
		ret.Group = "core"
		ret.Version = parts[0]
		ret.Kind = parts[1]
	default:
		return ret, errors.New("invalid value")
	}

	return ret, nil
}
