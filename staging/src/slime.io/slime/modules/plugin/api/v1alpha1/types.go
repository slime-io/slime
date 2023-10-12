package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true

// EnvoyPlugin is the Schema for the EnvoyPlugin API
type EnvoyPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvoyPluginSpec   `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// EnvoyPluginList contains a list of EnvoyPlugin
type EnvoyPluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnvoyPlugin `json:"items"`
}

//+kubebuilder:object:root=true

// PluginManager is the Schema for the PluginManager API
type PluginManager struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PluginManagerSpec   `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// PluginManagerList contains a list of PluginManager
type PluginManagerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PluginManager `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EnvoyPlugin{}, &EnvoyPluginList{})
	SchemeBuilder.Register(&PluginManager{}, &PluginManagerList{})
}
