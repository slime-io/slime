package kube

import "k8s.io/apimachinery/pkg/runtime/schema"

var (
	ServiceGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}
	ConfigMapGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
	EnvoyFilterGVR = schema.GroupVersionResource{
		Group:    "networking.istio.io",
		Version:  "v1alpha3",
		Resource: "envoyfilters",
	}
)

var (
	ServiceGVK = schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	}
	ConfigMapGVK = schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}
	EnvoyFilterGVK = schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1alpha3",
		Kind:    "EnvoyFilter",
	}
)

var (
	gvr2GVK = map[schema.GroupVersionResource]schema.GroupVersionKind{
		ServiceGVR:     ServiceGVK,
		ConfigMapGVR:   ConfigMapGVK,
		EnvoyFilterGVR: EnvoyFilterGVK,
	}

	gvk2GVR = map[schema.GroupVersionKind]schema.GroupVersionResource{
		ServiceGVK:     ServiceGVR,
		ConfigMapGVK:   ConfigMapGVR,
		EnvoyFilterGVK: EnvoyFilterGVR,
	}
)

func ConvertToGroupVersionResource(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	return gvk2GVR[gvk]
}

func ConvertToGroupVersionKind(gvr schema.GroupVersionResource) schema.GroupVersionKind {
	return gvr2GVK[gvr]
}
