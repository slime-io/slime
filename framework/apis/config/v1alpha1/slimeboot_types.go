package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type SlimeBoot struct {
	metav1.TypeMeta   `json:",inline,omitempty" yaml:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec              *SlimeBootSpec `json:"spec" yaml:"spec"`
}

type SlimeBootList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlimeBoot `json:"items"`
}
