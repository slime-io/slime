/*
Copyright 2023 slime.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	RegistrySourcesResource = GroupVersion.WithResource("registrysources")
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type DubboInterface struct {
	Interface string `json:"interface,omitempty"`
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
}

// Zookeeper all about zookeeper registry
type Zookeeper struct {
	// AvailableInterfaces is the list of interfaces that are available in the zk registry
	// +optional
	AvailableInterfaces []DubboInterface `json:"availableInterfaces,omitempty"`
	// GlobalAbnormalInstanceIPs
	//   - key: name of the ip sets
	//   - value: abnormal instance ip list
	// +optional
	GlobalAbnormalInstanceIPs map[string][]string `json:"globalAbnormalInstanceIPs,omitempty"`
	// AbnormalInstanceIPs
	//   - key: name of the dubbo interface, format: interface:group:version
	//   - value: abnormal instance ip list of the specified interface
	// +optional
	AbnormalInstanceIPs map[string][]string `json:"AbnormalInstanceIPs,omitempty"`
}

// RegistrySourceSpec defines the desired state of RegistrySource
type RegistrySourceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Zookeeper *Zookeeper `json:"zookeeper,omitempty"`
}

// RegistrySourceStatus defines the observed state of RegistrySource
type RegistrySourceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RegistrySource is the Schema for the RegistrySource API
type RegistrySource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegistrySourceSpec   `json:"spec,omitempty"`
	Status RegistrySourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RegistrySourceList contains a list of RegistrySource
type RegistrySourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RegistrySource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RegistrySource{}, &RegistrySourceList{})
}
