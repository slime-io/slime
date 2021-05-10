package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceFence is the Schema for the servicefences API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=servicefences,scope=Namespaced
type ServiceFence struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceFenceSpec   `json:"spec,omitempty"`
	Status ServiceFenceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceFenceList contains a list of ServiceFence
type ServiceFenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceFence `json:"items"`
}

// GetSpec from a wrapper
func (in *ServiceFence) GetSpec() map[string]interface{} {
	return nil
}

// GetObjectMeta from a wrapper
func (in *ServiceFence) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func init() {
	SchemeBuilder.Register(&ServiceFence{}, &ServiceFenceList{})
}
