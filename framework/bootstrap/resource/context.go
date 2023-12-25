package resource

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// NodeMetadata defines the metadata associated with a proxy
// Fields should not be assumed to exist on the proxy, especially newly added fields which will not exist
// on older versions.
// The JSON field names should never change, as they are needed for backward compatibility with older proxies
// nolint: maligned
type NodeMetadata struct {
	// IstioVersion specifies the Istio version associated with the proxy
	IstioVersion string `json:"ISTIO_VERSION,omitempty"`

	// IstioRevision specifies the Istio revision associated with the proxy.
	// Mostly used when istiod requests the upstream.
	IstioRevision string `json:"ISTIO_REVISION,omitempty"`

	// Labels specifies the set of workload instance (ex: k8s pod) labels associated with this node.
	Labels map[string]string `json:"LABELS,omitempty"`

	// Namespace is the namespace in which the workload instance is running.
	Namespace string `json:"NAMESPACE,omitempty"`

	// Generator indicates the client wants to use a custom Generator plugin.
	Generator string `json:"GENERATOR,omitempty"`
}

// ToStruct Converts this to a protobuf structure. This should be used only for debugging - performance is bad.
func (m NodeMetadata) ToStruct() *structpb.Struct {
	j, err := json.Marshal(m)
	if err != nil {
		return nil
	}

	pbs := &structpb.Struct{}
	if err := protojson.Unmarshal(j, pbs); err != nil {
		return nil
	}

	return pbs
}
