package resource

import (
	"time"

	"github.com/gogo/protobuf/proto"
)

var (
	ServiceEntry  = GroupVersionKind{Group: "networking.istio.io", Kind: "ServiceEntry", Version: "v1alpha3"}
	Service       = GroupVersionKind{Group: "core", Version: "v1", Kind: "Service"}
	Endpoints     = GroupVersionKind{Group: "core", Version: "v1", Kind: "Endpoints"}
	Pod           = GroupVersionKind{Group: "core", Version: "v1", Kind: "Pod"}
	ConfigMap     = GroupVersionKind{Group: "core", Version: "v1", Kind: "ConfigMap"}
	IstioService  = GroupVersionKind{Group: "networking.istio.io", Version: "v1alpha3", Kind: "IstioService"}
	IstioEndpoint = GroupVersionKind{Group: "networking.istio.io", Version: "v1alpha3", Kind: "IstioEndpoint"}
)

type GroupVersionKind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

func (g *GroupVersionKind) String() string {
	if g.Group == "" {
		return "core/" + g.Version + "/" + g.Kind
	}
	return g.Group + "/" + g.Version + "/" + g.Kind
}

type Config struct {
	ConfigMeta
	// Spec holds the configuration object as a gogo protobuf message
	Spec proto.Message
}

// ConfigMeta is metadata attached to each configuration unit.
type ConfigMeta struct {
	GroupVersionKind  GroupVersionKind  `json:"type,omitempty"`
	Name              string            `json:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	Domain            string            `json:"domain,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
}
