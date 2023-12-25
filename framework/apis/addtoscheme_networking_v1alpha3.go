package apis

import (
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, networkingv1alpha3.SchemeBuilder.AddToScheme)
}
