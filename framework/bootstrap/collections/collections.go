package collections

import (
	"reflect"

	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/libistio/pkg/config/schema/resource"
	"istio.io/libistio/pkg/config/validation"

	"slime.io/slime/framework/bootstrap/serviceregistry/model"
)

var (
	IstioServices = resource.Builder{
		Identifier:    "IstioService",
		Group:         "networking.istio.io",
		Kind:          "IstioService",
		Plural:        "istioservices",
		Version:       "v1alpha3",
		Proto:         "",
		ReflectType:   reflect.TypeOf(&model.Service{}).Elem(),
		ProtoPackage:  "",
		ValidateProto: validation.EmptyValidate,
	}.MustBuild()

	IstioEndpoints = resource.Builder{
		Group:         "networking.istio.io",
		Kind:          "IstioEndpoint",
		Plural:        "istioendpoints",
		Version:       "v1alpha3",
		Proto:         "",
		ReflectType:   reflect.TypeOf(&model.IstioEndpoint{}).Elem(),
		ProtoPackage:  "",
		ValidateProto: validation.EmptyValidate,
	}.MustBuild()

	Istio = collection.NewSchemasBuilder().
		MustAdd(IstioServices).
		MustAdd(IstioEndpoints).
		Build()

	Kube  = collections.Kube
	Pilot = collections.Pilot
)
