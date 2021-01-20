package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SlimeBoot is the Schema for the slimeboots API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=slimeboots,scope=Namespaced
type SlimeBoot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SlimeBootSpec   `json:"spec,omitempty"`
	Status SlimeBootStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SlimeBootList contains a list of SlimeBoot
type SlimeBootList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlimeBoot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SlimeBoot{}, &SlimeBootList{})
}
