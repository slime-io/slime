package collections

import (
	"google.golang.org/protobuf/proto"

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

	// IstioExtensionsV1Alpha1Riderplugins describes the collection
	// istio/extensions/v1alpha1/riderplugins
	IstioExtensionsV1Alpha1Riderplugins = resource.Builder{
		Name:         "istio/extensions/v1alpha1/riderplugins",
		VariableName: "IstioExtensionsV1Alpha1Riderplugins",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "extensions.istio.io",
			Kind:          "RiderPlugin",
			Plural:        "riderplugins",
			Version:       "v1alpha1",
			Proto:         "istio.extensions.v1alpha1.RiderPlugin",
			ProtoPackage:  "istio.io/api/extensions/v1alpha1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioExtensionsV1Alpha1Wasmplugins describes the collection
	// istio/extensions/v1alpha1/wasmplugins
	IstioExtensionsV1Alpha1Wasmplugins = resource.Builder{
		Name:         "istio/extensions/v1alpha1/wasmplugins",
		VariableName: "IstioExtensionsV1Alpha1Wasmplugins",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "extensions.istio.io",
			Kind:          "WasmPlugin",
			Plural:        "wasmplugins",
			Version:       "v1alpha1",
			Proto:         "istio.extensions.v1alpha1.WasmPlugin",
			ProtoPackage:  "istio.io/api/extensions/v1alpha1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Destinationrules describes the collection
	// istio/networking/v1alpha3/destinationrules
	IstioNetworkingV1Alpha3Destinationrules = resource.Builder{
		Name:         "istio/networking/v1alpha3/destinationrules",
		VariableName: "IstioNetworkingV1Alpha3Destinationrules",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "DestinationRule",
			Plural:        "destinationrules",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.DestinationRule",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Envoyfilters describes the collection
	// istio/networking/v1alpha3/envoyfilters
	IstioNetworkingV1Alpha3Envoyfilters = resource.Builder{
		Name:         "istio/networking/v1alpha3/envoyfilters",
		VariableName: "IstioNetworkingV1Alpha3Envoyfilters",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "EnvoyFilter",
			Plural:        "envoyfilters",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.EnvoyFilter",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Gateways describes the collection
	// istio/networking/v1alpha3/gateways
	IstioNetworkingV1Alpha3Gateways = resource.Builder{
		Name:         "istio/networking/v1alpha3/gateways",
		VariableName: "IstioNetworkingV1Alpha3Gateways",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "Gateway",
			Plural:        "gateways",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.Gateway",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Sidecars describes the collection
	// istio/networking/v1alpha3/sidecars
	IstioNetworkingV1Alpha3Sidecars = resource.Builder{
		Name:         "istio/networking/v1alpha3/sidecars",
		VariableName: "IstioNetworkingV1Alpha3Sidecars",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "Sidecar",
			Plural:        "sidecars",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.Sidecar",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Virtualservices describes the collection
	// istio/networking/v1alpha3/virtualservices
	IstioNetworkingV1Alpha3Virtualservices = resource.Builder{
		Name:         "istio/networking/v1alpha3/virtualservices",
		VariableName: "IstioNetworkingV1Alpha3Virtualservices",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "VirtualService",
			Plural:        "virtualservices",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.VirtualService",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Workloadentries describes the collection
	// istio/networking/v1alpha3/workloadentries
	IstioNetworkingV1Alpha3Workloadentries = resource.Builder{
		Name:         "istio/networking/v1alpha3/workloadentries",
		VariableName: "IstioNetworkingV1Alpha3Workloadentries",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "WorkloadEntry",
			Plural:        "workloadentries",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.WorkloadEntry",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioNetworkingV1Alpha3Workloadgroups describes the collection
	// istio/networking/v1alpha3/workloadgroups
	IstioNetworkingV1Alpha3Workloadgroups = resource.Builder{
		Name:         "istio/networking/v1alpha3/workloadgroups",
		VariableName: "IstioNetworkingV1Alpha3Workloadgroups",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "networking.istio.io",
			Kind:          "WorkloadGroup",
			Plural:        "workloadgroups",
			Version:       "v1alpha3",
			Proto:         "istio.networking.v1alpha3.WorkloadGroup",
			ProtoPackage:  "istio.io/api/networking/v1alpha3",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioSecurityV1Beta1Authorizationpolicies describes the collection
	// istio/security/v1beta1/authorizationpolicies
	IstioSecurityV1Beta1Authorizationpolicies = resource.Builder{
		Name:         "istio/security/v1beta1/authorizationpolicies",
		VariableName: "IstioSecurityV1Beta1Authorizationpolicies",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "security.istio.io",
			Kind:          "AuthorizationPolicy",
			Plural:        "authorizationpolicies",
			Version:       "v1beta1",
			Proto:         "istio.security.v1beta1.AuthorizationPolicy",
			ProtoPackage:  "istio.io/api/security/v1beta1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioSecurityV1Beta1Peerauthentications describes the collection
	// istio/security/v1beta1/peerauthentications
	IstioSecurityV1Beta1Peerauthentications = resource.Builder{
		Name:         "istio/security/v1beta1/peerauthentications",
		VariableName: "IstioSecurityV1Beta1Peerauthentications",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "security.istio.io",
			Kind:          "PeerAuthentication",
			Plural:        "peerauthentications",
			Version:       "v1beta1",
			Proto:         "istio.security.v1beta1.PeerAuthentication",
			ProtoPackage:  "istio.io/api/security/v1beta1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioSecurityV1Beta1Requestauthentications describes the collection
	// istio/security/v1beta1/requestauthentications
	IstioSecurityV1Beta1Requestauthentications = resource.Builder{
		Name:         "istio/security/v1beta1/requestauthentications",
		VariableName: "IstioSecurityV1Beta1Requestauthentications",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "security.istio.io",
			Kind:          "RequestAuthentication",
			Plural:        "requestauthentications",
			Version:       "v1beta1",
			Proto:         "istio.security.v1beta1.RequestAuthentication",
			ProtoPackage:  "istio.io/api/security/v1beta1",
			ClusterScoped: false,
			ValidateProto: EmptyValidate,
		}.MustBuild(),
	}.MustBuild()

	// IstioTelemetryV1Alpha1Telemetries describes the collection
	// istio/telemetry/v1alpha1/telemetries
	IstioTelemetryV1Alpha1Telemetries = resource.Builder{
		Name:         "istio/telemetry/v1alpha1/telemetries",
		VariableName: "IstioTelemetryV1Alpha1Telemetries",
		Disabled:     false,
		Resource: resource.SubBuilder{
			Group:         "telemetry.istio.io",
			Kind:          "Telemetry",
			Plural:        "telemetries",
			Version:       "v1alpha1",
			Proto:         "istio.telemetry.v1alpha1.Telemetry",
			ProtoPackage:  "istio.io/api/telemetry/v1alpha1",
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
		MustAdd(IstioExtensionsV1Alpha1Riderplugins).
		MustAdd(IstioExtensionsV1Alpha1Wasmplugins).
		MustAdd(IstioNetworkingV1Alpha3Destinationrules).
		MustAdd(IstioNetworkingV1Alpha3Envoyfilters).
		MustAdd(IstioNetworkingV1Alpha3Gateways).
		MustAdd(IstioNetworkingV1Alpha3Serviceentries).
		MustAdd(IstioNetworkingV1Alpha3Sidecars).
		MustAdd(IstioNetworkingV1Alpha3Virtualservices).
		MustAdd(IstioNetworkingV1Alpha3Workloadentries).
		MustAdd(IstioNetworkingV1Alpha3Workloadgroups).
		MustAdd(IstioSecurityV1Beta1Authorizationpolicies).
		MustAdd(IstioSecurityV1Beta1Peerauthentications).
		MustAdd(IstioSecurityV1Beta1Requestauthentications).
		MustAdd(IstioTelemetryV1Alpha1Telemetries).
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
