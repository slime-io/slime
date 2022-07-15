package collections

import (
	"github.com/gogo/protobuf/proto"
	"slime.io/slime/framework/bootstrap/resource"
)

var (
	EmptyValidate = func(string, string, proto.Message) error {
		return nil
	}

	// IstioMeshV1Alpha1MeshConfig describes the collection
	// istio/mesh/v1alpha1/MeshConfig
	IstioMeshV1Alpha1MeshConfig = resource.Builder{
		Name:         "istio/mesh/v1alpha1/MeshConfig",
		VariableName: "IstioMeshV1Alpha1MeshConfig",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "",
			Kind:          "MeshConfig",
			Plural:        "meshconfigs",
			Version:       "v1alpha1",
			Proto:         "istio.mesh.v1alpha1.MeshConfig",
			ProtoPackage:  "istio.io/api/mesh/v1alpha1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Serviceentries describes the collection
	// istio/networking/v1alpha3/serviceentries
	IstioNetworkingV1Alpha3Serviceentries = resource.Builder{
		Name:         "istio/networking/v1alpha3/serviceentries",
		VariableName: "IstioNetworkingV1Alpha3Serviceentries",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "ServiceEntry",
			Plural:        "serviceentries",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.ServiceEntry",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// K8SCoreV1Configmaps describes the collection k8s/core/v1/configmaps
	K8SCoreV1Configmaps = resource.Builder{
		Name:         "k8s/core/v1/configmaps",
		VariableName: "K8SCoreV1Configmaps",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "core",
			Kind:          "ConfigMap",
			Plural:        "configmaps",
			Version:       "v1",
			Proto:         "k8s.io.api.core.v1.ConfigMap",
			ProtoPackage:  "k8s.io/api/core/v1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// K8SCoreV1Endpoints describes the collection k8s/core/v1/endpoints
	K8SCoreV1Endpoints = resource.Builder{
		Name:         "k8s/core/v1/endpoints",
		VariableName: "K8SCoreV1Endpoints",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "core",
			Kind:          "Endpoints",
			Plural:        "endpoints",
			Version:       "v1",
			Proto:         "k8s.io.api.core.v1.Endpoints",
			ProtoPackage:  "k8s.io/api/core/v1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// K8SCoreV1Pods describes the collection k8s/core/v1/pods
	K8SCoreV1Pods = resource.Builder{
		Name:         "k8s/core/v1/pods",
		VariableName: "K8SCoreV1Pods",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "core",
			Kind:          "Pod",
			Plural:        "pods",
			Version:       "v1",
			Proto:         "k8s.io.api.core.v1.Pod",
			ProtoPackage:  "k8s.io/api/core/v1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// K8SCoreV1Services describes the collection k8s/core/v1/services
	K8SCoreV1Services = resource.Builder{
		Name:         "k8s/core/v1/services",
		VariableName: "K8SCoreV1Services",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "core",
			Kind:          "Service",
			Plural:        "services",
			Version:       "v1",
			Proto:         "k8s.io.api.core.v1.ServiceSpec",
			ProtoPackage:  "k8s.io/api/core/v1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	IstioServices = resource.Builder{
		Name:         "istio/networking/v1alpha3/istioservices",
		VariableName: "IstioNetworkingV1Alpha3Istioservices",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "IstioService",
			Plural:        "istioservices",
			Version:       "v1alpha3",
			Proto:         "",
			ProtoPackage:  "",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	IstioEndpoints = resource.Builder{
		Name:         "istio/networking/v1alpha3/istioendpoints",
		VariableName: "IstioNetworkingV1Alpha3Istioendpoints",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "IstioEndpoint",
			Plural:        "istioendpoints",
			Version:       "v1alpha3",
			Proto:         "",
			ProtoPackage:  "",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// Pilot contains only collections used by Pilot.
	Pilot = resource.NewSchemasBuilder().
		MustAdd(IstioNetworkingV1Alpha3Serviceentries).
		Build()

	// Kube contains only kubernetes collections.
	Kube = resource.NewSchemasBuilder().
		MustAdd(K8SCoreV1Configmaps).
		MustAdd(K8SCoreV1Endpoints).
		MustAdd(K8SCoreV1Pods).
		MustAdd(K8SCoreV1Services).
		MustAdd(IstioNetworkingV1Alpha3Serviceentries).
		Build()

	Istio = resource.NewSchemasBuilder().
		MustAdd(IstioServices).
		MustAdd(IstioEndpoints).
		Build()
)
