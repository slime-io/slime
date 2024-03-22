package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:resource:shortName=slb
// +kubebuilder:subresource:status
type SlimeBoot struct {
	metav1.TypeMeta   `json:",inline,omitempty" yaml:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Spec *SlimeBootSpec `json:"spec" yaml:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Status *SlimeBootStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type SlimeBootList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlimeBoot `json:"items"`
}
