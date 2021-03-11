/*
* @Author: yangdihang
* @Date: 2020/6/8
 */

package envoyplugin

import (
	"fmt"
	"strings"

	"slime.io/slime/pkg/apis/microservice/v1alpha1"
	"slime.io/slime/pkg/util"

	"github.com/gogo/protobuf/types"
	istio "istio.io/api/networking/v1alpha3"
)

func translatePluginToPatch(name, typeurl string, setting *types.Struct) *istio.EnvoyFilter_Patch {
	patch := &istio.EnvoyFilter_Patch{}
	patch.Value = &types.Struct{
		Fields: map[string]*types.Value{
			util.Struct_HttpFilter_TypedPerFilterConfig: {
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{
							name: {
								Kind: &types.Value_StructValue{StructValue: &types.Struct{
									Fields: map[string]*types.Value{
										util.Struct_Any_Value: {
											Kind: &types.Value_StructValue{StructValue: setting},
										},
										util.Struct_Any_TypedUrl: {
											Kind: &types.Value_StringValue{StringValue: typeurl},
										},
										util.Struct_Any_AtType: {
											Kind: &types.Value_StringValue{StringValue: util.TypeUrl_UdpaTypedStruct},
										},
									},
								}},
							},
						},
					},
				},
			},
		},
	}
	return patch
}

func translateEnvoyPlugin(in *v1alpha1.EnvoyPlugin, out *istio.EnvoyFilter) {
	if in.WorkloadSelector != nil {
		out.WorkloadSelector = &istio.WorkloadSelector{
			Labels: in.WorkloadSelector.Labels,
		}
	}
	out.ConfigPatches = make([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, 0)

	for _, h := range in.Host {
		for _, p := range in.Plugins {
			if p.PluginSettings == nil {
				log.Error(fmt.Errorf("empty setting"), "cause error happend, skip plugin build, plugin: "+p.Name)
			}
			var cfp *istio.EnvoyFilter_EnvoyConfigObjectPatch
			switch m := p.PluginSettings.(type) {
			case *v1alpha1.Plugin_Wasm:
				log.Error(fmt.Errorf("implentment"), "cause wasm not been support in envoyplugin settings, skip plugin build, plugin: "+p.Name)
			case *v1alpha1.Plugin_Inline:
				cfp = &istio.EnvoyFilter_EnvoyConfigObjectPatch{
					ApplyTo: istio.EnvoyFilter_VIRTUAL_HOST,
					Match: &istio.EnvoyFilter_EnvoyConfigObjectMatch{
						ObjectTypes: &istio.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
							RouteConfiguration: &istio.EnvoyFilter_RouteConfigurationMatch{
								Vhost: &istio.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{
									Name: h,
								},
							},
						},
					},
				}
				if p.Name == util.Envoy_Ratelimit || p.Name == util.Envoy_Cors {
					cfp.Patch = translateRatelimitToPatch(m.Inline.Settings)
				} else {
					cfp.Patch = translatePluginToPatch(p.Name, p.TypeUrl, m.Inline.Settings)
				}
				cfp.Patch.Operation = istio.EnvoyFilter_Patch_MERGE
			}
			out.ConfigPatches = append(out.ConfigPatches, cfp)
		}
	}

	for _, route := range in.Route {
		ss := strings.SplitN(route, "/", 2)
		if len(ss) != 2 {
			// patch to all host
			ss = []string{"", ss[0]}
		}
		for _, p := range in.Plugins {
			if p.PluginSettings == nil {
				log.Error(fmt.Errorf("empty setting"), "cause error happend, skip plugin build, plugin: "+p.Name)
			}
			var cfp *istio.EnvoyFilter_EnvoyConfigObjectPatch
			switch m := p.PluginSettings.(type) {
			case *v1alpha1.Plugin_Wasm:
				log.Error(fmt.Errorf("implentment"), "cause wasm not been support in envoyplugin settings, skip plugin build, plugin: "+p.Name)
			case *v1alpha1.Plugin_Inline:
				cfp = &istio.EnvoyFilter_EnvoyConfigObjectPatch{
					ApplyTo: istio.EnvoyFilter_HTTP_ROUTE,
					Match: &istio.EnvoyFilter_EnvoyConfigObjectMatch{
						ObjectTypes: &istio.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
							RouteConfiguration: &istio.EnvoyFilter_RouteConfigurationMatch{
								Vhost: &istio.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{
									Route: &istio.EnvoyFilter_RouteConfigurationMatch_RouteMatch{
										Name: ss[1],
									},
								},
							},
						},
					},
				}
				if ss[0] != "" {
					cfp.Match.ObjectTypes.(*istio.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration).RouteConfiguration.Vhost.Name = ss[0]
				}
				if p.Name == util.Envoy_Ratelimit || p.Name == util.Envoy_Cors {
					cfp.Patch = translateRatelimitToPatch(m.Inline.Settings)
				} else {
					cfp.Patch = translatePluginToPatch(p.Name, p.TypeUrl, m.Inline.Settings)
				}
				cfp.Patch.Operation = istio.EnvoyFilter_Patch_MERGE
			}

			out.ConfigPatches = append(out.ConfigPatches, cfp)
		}
	}
}

func translateRatelimitToPatch(settings *types.Struct) *istio.EnvoyFilter_Patch {
	patch := &istio.EnvoyFilter_Patch{}
	patch.Value = settings
	return patch
}
