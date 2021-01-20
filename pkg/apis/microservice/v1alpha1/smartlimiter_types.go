package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SmartLimiter is the Schema for the smartlimiters API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=smartlimiters,scope=Namespaced
type SmartLimiter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SmartLimiterSpec   `json:"spec,omitempty"`
	Status SmartLimiterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SmartLimiterList contains a list of SmartLimiter
type SmartLimiterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SmartLimiter `json:"items"`
}

// GetSpec from a wrapper
func (in *SmartLimiter) GetSpec() map[string]interface{} {
	return nil
}

// GetObjectMeta from a wrapper
func (in *SmartLimiter) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func init() {
	SchemeBuilder.Register(&SmartLimiter{}, &SmartLimiterList{})
}
