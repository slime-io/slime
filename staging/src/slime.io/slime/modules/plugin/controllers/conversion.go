/*
* @Author: yangdihang
* @Date: 2020/6/8
 */

package controllers

import (
	"fmt"
	"strings"

	"slime.io/slime/framework/util"
	"slime.io/slime/modules/plugin/api/v1alpha1"
	microserviceslimeiov1alpha1types "slime.io/slime/modules/plugin/api/v1alpha1"
	microserviceslimeiov1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1/wrapper"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_wasm_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"github.com/gogo/protobuf/types"
	istio "istio.io/api/networking/v1alpha3"
)

// genGatewayCfps is a custom func to handle EnvoyPlugin gateway
// default is nil, ignore gateway
var genGatewayCfps func(in *microserviceslimeiov1alpha1types.EnvoyPlugin, namespace string, t target, patchCtx istio.EnvoyFilter_PatchContext,
	p *microserviceslimeiov1alpha1types.Plugin, m *v1alpha1.Plugin_Inline) []*istio.EnvoyFilter_EnvoyConfigObjectPatch

type target struct {
	applyTo     istio.EnvoyFilter_ApplyTo
	host, route string
}

var (
	directPatchingPlugins = []string{
		util.EnvoyHTTPRateLimit,
		util.EnvoyCors,
		util.EnvoyRatelimitV1, // keep backward compatibility
	}
)

func directPatching(name string) bool {
	for _, plugin := range directPatchingPlugins {
		if name == plugin {
			return true
		}
	}
	return false
}

// translate EnvoyPlugin
func translatePluginToPatch(name, typeurl string, setting *types.Struct) *istio.EnvoyFilter_Patch {
	patch := &istio.EnvoyFilter_Patch{}
	patch.Value = &types.Struct{
		Fields: map[string]*types.Value{
			util.StructHttpFilterTypedPerFilterConfig: {
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{
							name: {
								Kind: &types.Value_StructValue{StructValue: &types.Struct{
									Fields: map[string]*types.Value{
										util.StructAnyValue: {
											Kind: &types.Value_StructValue{StructValue: setting},
										},
										util.StructAnyTypedURL: {
											Kind: &types.Value_StringValue{StringValue: typeurl},
										},
										util.StructAnyAtType: {
											Kind: &types.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
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

func translateRlsAndCorsToDirectPatch(settings *types.Struct, applyToHTTPRoute bool) *istio.EnvoyFilter_Patch {
	fieldPatchTo := ""
	if applyToHTTPRoute {
		fieldPatchTo = "route"
	}
	return translatePluginToDirectPatch(settings, fieldPatchTo)
}

func translatePluginToDirectPatch(settings *types.Struct, fieldPatchTo string) *istio.EnvoyFilter_Patch {
	patch := &istio.EnvoyFilter_Patch{}

	if fieldPatchTo == "ROOT" {
		fieldPatchTo = ""
	}

	if fieldPatchTo != "" {
		patch.Value = &types.Struct{
			Fields: map[string]*types.Value{
				fieldPatchTo: {
					Kind: &types.Value_StructValue{
						StructValue: settings,
					},
				},
			},
		}
	} else {
		patch.Value = settings
	}
	return patch
}

func (r *EnvoyPluginReconciler) translateEnvoyPlugin(cr *microserviceslimeiov1alpha1.EnvoyPlugin, out *istio.EnvoyFilter) {
	pb, err := util.FromJSONMap("slime.microservice.plugin.v1alpha1.EnvoyPlugin", cr.Spec)
	if err != nil {
		log.Errorf("unable to convert envoyPlugin to envoyFilter,%+v", err)
		return
	}
	in := pb.(*microserviceslimeiov1alpha1types.EnvoyPlugin)

	if in.WorkloadSelector != nil {
		out.WorkloadSelector = &istio.WorkloadSelector{
			Labels: in.WorkloadSelector.Labels,
		}
	}
	out.ConfigPatches = make([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, 0)

	var targets []target
	for _, h := range in.Host {
		targets = append(targets, target{
			applyTo: istio.EnvoyFilter_VIRTUAL_HOST,
			host:    h,
		})
	}
	for _, fullRoute := range in.Route {
		host, route := "", fullRoute
		if ss := strings.SplitN(fullRoute, "/", 2); len(ss) == 2 {
			host, route = ss[0], ss[1]
		}

		targets = append(targets, target{
			applyTo: istio.EnvoyFilter_HTTP_ROUTE,
			host:    host,
			route:   route,
		})
	}

	for _, t := range targets {
		for _, p := range in.Plugins {
			if !p.Enable {
				continue
			}

			if p.PluginSettings == nil {
				log.Errorf("empty setting, cause error happend, skip plugin build, plugin: %s", p.Name)
				continue
			}

			patchCtx := istio.EnvoyFilter_ANY
			if !strings.HasPrefix(t.host, "inbound|") { // keep backward compatibility
				switch p.ListenerType {
				case v1alpha1.Plugin_Outbound:
					patchCtx = istio.EnvoyFilter_SIDECAR_OUTBOUND
				case v1alpha1.Plugin_Inbound:
					patchCtx = istio.EnvoyFilter_SIDECAR_INBOUND
				case v1alpha1.Plugin_Gateway:
					patchCtx = istio.EnvoyFilter_GATEWAY
				}
			}

			switch m := p.PluginSettings.(type) {
			case *v1alpha1.Plugin_Wasm:
				log.Errorf("implentment, cause wasm not been support in envoyplugin settings, skip plugin build, plugin: %s")
				continue
			case *v1alpha1.Plugin_Inline:
				if len(in.Gateway) > 0 && genGatewayCfps != nil {
					cfps := genGatewayCfps(in, cr.Namespace, t, patchCtx, p, m)
					out.ConfigPatches = append(out.ConfigPatches, cfps...)
				} else {
					vhost := &istio.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{
						Name: t.host,
					}
					if t.applyTo == istio.EnvoyFilter_HTTP_ROUTE {
						vhost.Route = &istio.EnvoyFilter_RouteConfigurationMatch_RouteMatch{
							Name: t.route,
						}
					}
					cfp := generateCfp(t, patchCtx, vhost, p, m)
					out.ConfigPatches = append(out.ConfigPatches, cfp)
				}
			}
		}
	}
	log.Debugf("translate EnvoyPlugin to Envoyfilter: %v", out)
}

func generateCfp(t target, patchCtx istio.EnvoyFilter_PatchContext, vhost *istio.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch,
	p *microserviceslimeiov1alpha1types.Plugin, m *v1alpha1.Plugin_Inline) *istio.EnvoyFilter_EnvoyConfigObjectPatch {
	cfp := &istio.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: t.applyTo,
		Match: &istio.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: patchCtx,
			ObjectTypes: &istio.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
				RouteConfiguration: &istio.EnvoyFilter_RouteConfigurationMatch{
					Vhost: vhost,
				},
			},
		},
	}

	if directPatching(p.Name) {
		cfp.Patch = translateRlsAndCorsToDirectPatch(m.Inline.Settings, t.applyTo == istio.EnvoyFilter_HTTP_ROUTE)
	} else if m.Inline.DirectPatch {
		cfp.Patch = translatePluginToDirectPatch(m.Inline.Settings, m.Inline.FieldPatchTo)
	} else {
		cfp.Patch = translatePluginToPatch(p.Name, p.TypeUrl, m.Inline.Settings)
	}

	cfp.Patch.Operation = istio.EnvoyFilter_Patch_MERGE
	return cfp
}

// translate PluginManager
func (r *PluginManagerReconciler) translatePluginManager(in *v1alpha1.PluginManager, out *istio.EnvoyFilter) {
	out.WorkloadSelector = &istio.WorkloadSelector{
		Labels: in.WorkloadLabels,
	}
	out.ConfigPatches = make([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	for _, p := range in.Plugin {
		if !p.Enable {
			continue
		}
		patches, err := r.convertPluginToPatch(p)
		if err != nil {
			log.Errorf("cause error happened, skip plugin build, plugin: %s, %+v", p.Name, err)
			continue
		}

		out.ConfigPatches = append(out.ConfigPatches, patches...)
	}
}

func (r *PluginManagerReconciler) convertPluginToPatch(in *v1alpha1.Plugin) ([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	listener := &istio.EnvoyFilter_ListenerMatch{
		FilterChain: &istio.EnvoyFilter_ListenerMatch_FilterChainMatch{
			Filter: &istio.EnvoyFilter_ListenerMatch_FilterMatch{
				Name: util.EnvoyHTTPConnectionManager,
				SubFilter: &istio.EnvoyFilter_ListenerMatch_SubFilterMatch{
					Name: util.EnvoyRoute,
				},
			},
		},
	}

	if in.Port != 0 {
		listener.PortNumber = in.Port
	}

	out := &istio.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: istio.EnvoyFilter_HTTP_FILTER,
		Match: &istio.EnvoyFilter_EnvoyConfigObjectMatch{
			ObjectTypes: &istio.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
				Listener: listener,
			},
		},
		Patch: &istio.EnvoyFilter_Patch{
			Operation: istio.EnvoyFilter_Patch_INSERT_BEFORE,
			Value: &types.Struct{
				Fields: map[string]*types.Value{},
			},
		},
	}

	ret := []*istio.EnvoyFilter_EnvoyConfigObjectPatch{out}

	switch in.ListenerType {
	case v1alpha1.Plugin_Outbound:
		out.Match.Context = istio.EnvoyFilter_SIDECAR_OUTBOUND
	case v1alpha1.Plugin_Inbound:
		out.Match.Context = istio.EnvoyFilter_SIDECAR_INBOUND
	case v1alpha1.Plugin_Gateway:
		out.Match.Context = istio.EnvoyFilter_GATEWAY
	}

	if in.PluginSettings == nil {
		if err := r.applyInlinePlugin(in, nil, out.Patch.Value); err != nil {
			return nil, err
		}
		return ret, nil
	}

	switch m := in.PluginSettings.(type) {
	case *v1alpha1.Plugin_Wasm:
		if err := r.applyWasmPlugin(in, m, out.Patch.Value); err != nil {
			return nil, err
		}
		// TODO add extension patch
	case *v1alpha1.Plugin_Inline:
		if err := r.applyInlinePlugin(in, m, out.Patch.Value); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (r *PluginManagerReconciler) applyInlinePlugin(in *v1alpha1.Plugin, settings *v1alpha1.Plugin_Inline, out *types.Struct) error {
	out.Fields[util.StructHttpFilterName] = &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: in.Name,
		},
	}

	if settings != nil {
		out.Fields[util.StructHttpFilterTypedConfig] = &types.Value{
			Kind: &types.Value_StructValue{
				StructValue: &types.Struct{
					Fields: map[string]*types.Value{
						util.StructAnyTypedURL: {
							Kind: &types.Value_StringValue{StringValue: in.TypeUrl},
						},
						util.StructAnyAtType: {
							Kind: &types.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
						},
						util.StructAnyValue: {
							Kind: &types.Value_StructValue{StructValue: settings.Inline.Settings},
						},
					},
				},
			},
		}
	}

	return nil
}

func (r *PluginManagerReconciler) applyWasmPlugin(in *v1alpha1.Plugin, settings *v1alpha1.Plugin_Wasm, out *types.Struct) error {
	out.Fields[util.StructWasmName] = &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: util.EnvoyFilterHTTPWasm,
		},
	}

	if settings.Wasm.RootID == "" {
		return fmt.Errorf("plugin:%s, wasm插件rootID丢失", in.Name)
	} else if settings.Wasm.FileName == "" {
		return fmt.Errorf("plugin: %s, wasm 文件缺失", in.Name)
	}

	filepath := r.wasm.Get(settings.Wasm.FileName)
	pluginConfig := &envoy_extensions_wasm_v3.PluginConfig{
		Name:   in.Name,
		RootId: settings.Wasm.RootID,
		Vm: &envoy_extensions_wasm_v3.PluginConfig_VmConfig{
			VmConfig: &envoy_extensions_wasm_v3.VmConfig{
				VmId:    in.Name,
				Runtime: util.EnvoyWasmV8,
				Code: &envoy_config_core_v3.AsyncDataSource{
					Specifier: &envoy_config_core_v3.AsyncDataSource_Local{
						Local: &envoy_config_core_v3.DataSource{
							Specifier: &envoy_config_core_v3.DataSource_Filename{
								Filename: filepath,
							},
						},
					},
				},
			},
		},
	}

	wasmSettings, err := util.MessageToStruct(pluginConfig)
	if err != nil {
		return err
	}

	if settings.Wasm.Settings != nil {
		var (
			anyType  string
			anyValue *types.Value
		)

		// string类型的配置解析为 google.protobuf.StringValue
		if len(settings.Wasm.Settings.Fields) == 1 && settings.Wasm.Settings.Fields["_string"] != nil {
			parseTostring := settings.Wasm.Settings.Fields["_string"]
			if s, ok := parseTostring.Kind.(*types.Value_StringValue); ok {
				anyType = util.TypeURLStringValue
				anyValue = &types.Value{Kind: s}
			}
		}

		// 非string类型的配置解析为 "type.googleapis.com/udpa.type.v1.TypedStruct"
		if anyValue == nil {
			anyType = util.TypeURLUDPATypedStruct
			anyValue = &types.Value{Kind: &types.Value_StructValue{StructValue: settings.Wasm.Settings}}
		}

		wasmSettings.Fields[util.StructWasmConfiguration] = &types.Value{
			Kind: &types.Value_StructValue{
				StructValue: &types.Struct{
					Fields: map[string]*types.Value{
						util.StructAnyAtType: {
							Kind: &types.Value_StringValue{StringValue: anyType},
						},
						util.StructAnyValue: anyValue,
					},
				},
			},
		}
	}

	out.Fields[util.StructHttpFilterTypedConfig] = &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: map[string]*types.Value{
					util.StructAnyTypedURL: {
						Kind: &types.Value_StringValue{StringValue: util.TypeURLEnvoyFilterHTTPWasm},
					},
					util.StructAnyAtType: {
						Kind: &types.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
					},
					util.StructAnyValue: {
						Kind: &types.Value_StructValue{StructValue: &types.Struct{
							Fields: map[string]*types.Value{
								util.StructWasmConfig: {
									Kind: &types.Value_StructValue{
										StructValue: wasmSettings,
									},
								},
							},
						}},
					},
				},
			},
		},
	}

	return nil
}
