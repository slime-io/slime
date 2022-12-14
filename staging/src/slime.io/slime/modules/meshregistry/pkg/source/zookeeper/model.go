package zookeeper

import (
	"reflect"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/resource"
)

const (
	wildcardNamespace = "*"
	DubboNamespace    = "dubbo"
)

type ServiceEntryWithMeta struct {
	ServiceEntry *networking.ServiceEntry
	Meta         resource.Metadata
}

type SidecarWithMeta struct {
	Sidecar *networking.Sidecar
	Meta    resource.Metadata
}

func strMapEquals(m1, m2 resource.StringMap) bool {
	if len(m1) != len(m2) {
		return false
	}

	for k, v1 := range m1 {
		v2, ok := m2[k]
		if !ok || v2 != v1 {
			return false
		}
	}

	return true
}

// metadataEquals compares the meta of THE-SAME-ONE resource which means the id fields like schema/fullname should be
// same and out of the scope of comparison
func metadataEquals(m1, m2 resource.Metadata) bool {
	if !strMapEquals(m1.Labels, m2.Labels) || !strMapEquals(m1.Annotations, m2.Annotations) {
		return false
	}

	return true
}

func (scm SidecarWithMeta) Equals(o SidecarWithMeta) bool {
	if !metadataEquals(scm.Meta, o.Meta) {
		return false
	}

	if !reflect.DeepEqual(scm.Sidecar, o.Sidecar) {
		return false
	}

	return true
}

type DubboServiceInstance struct {
	Name                string                 `json:"name"`
	Id                  string                 `json:"id"`
	Address             string                 `json:"address"`
	Port                uint32                 `json:"port"`
	SslPort             string                 `json:"sslPort"`
	Payload             ServiceInstancePayload `json:"payload"`
	RegistrationTimeUTC int64                  `json:"registrationTimeUtc"`
	ServiceType         string                 `json:"serviceType"`
	UriSpec             string                 `json:"uriSpec"`
}

type ServiceInstancePayload struct {
	Id       string            `json:"id"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata"`
}
