/*
* @Author: yangdihang
* @Date: 2020/6/9
 */

package pluginmanager

import (
	"fmt"

	microservice "slime.io/slime/pkg/apis/microservice/v1alpha1"
	"slime.io/slime/pkg/util"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_wasm_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"github.com/gogo/protobuf/types"
	istio "istio.io/api/networking/v1alpha3"
)

func (r *ReconcilePluginManager) translatePluginManager(in *microservice.PluginManager, out *istio.EnvoyFilter) {
	out.WorkloadSelector = &istio.WorkloadSelector{
		Labels: in.WorkloadLabels,
	}
	out.ConfigPatches = make([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	for i := range in.Plugin {
		p := in.Plugin[len(in.Plugin)-i-1]
		patch, err := r.convertPluginToPatch(p)
		if err != nil {
			log.Error(err, "cause error happened, skip plugin build, plugin:"+p.Name)
			continue
		}
		out.ConfigPatches = append(out.ConfigPatches, patch)
	}
}

func (r *ReconcilePluginManager) convertPluginToPatch(in *microservice.Plugin) (*istio.EnvoyFilter_EnvoyConfigObjectPatch, error) {

	out := &istio.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: istio.EnvoyFilter_HTTP_FILTER,
		Match: &istio.EnvoyFilter_EnvoyConfigObjectMatch{
			ObjectTypes: &istio.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
				Listener: &istio.EnvoyFilter_ListenerMatch{
					FilterChain: &istio.EnvoyFilter_ListenerMatch_FilterChainMatch{
						Filter: &istio.EnvoyFilter_ListenerMatch_FilterMatch{
							Name: util.Envoy_HttpConnectionManager,
							SubFilter: &istio.EnvoyFilter_ListenerMatch_SubFilterMatch{
								Name: util.Envoy_Route,
							},
						},
					},
				},
			},
		},
		Patch: &istio.EnvoyFilter_Patch{
			Operation: istio.EnvoyFilter_Patch_INSERT_BEFORE,
			Value: &types.Struct{
				Fields: map[string]*types.Value{},
			},
		},
	}

	if in.ListenerType == microservice.Plugin_Inbound {
		out.Match.Context = istio.EnvoyFilter_SIDECAR_INBOUND
	} else {
		out.Match.Context = istio.EnvoyFilter_SIDECAR_OUTBOUND
	}

	var err error
	if in.PluginSettings != nil {
		switch m := in.PluginSettings.(type) {
		case *microservice.Plugin_Wasm:
			out.Patch.Value.Fields[util.Struct_Wasm_Name] = &types.Value{
				Kind: &types.Value_StringValue{
					StringValue: util.Envoy_FilterHttpWasm,
				},
			}

			if m.Wasm.RootID == "" {
				err = fmt.Errorf("plugin:%s, wasm插件rootID丢失", in.Name)
			} else if m.Wasm.FileName == "" {
				err = fmt.Errorf("plugin: %s, wasm 文件缺失", in.Name)
			} else {
				if err == nil {
					filepath := r.wasm.Get(m.Wasm.FileName)
					pluginConfig := &envoy_extensions_wasm_v3.PluginConfig{
						Name:   in.Name,
						RootId: m.Wasm.RootID,
						Vm: &envoy_extensions_wasm_v3.PluginConfig_VmConfig{
							VmConfig: &envoy_extensions_wasm_v3.VmConfig{
								VmId:    in.Name,
								Runtime: util.Envoy_WasmV8,
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
					settings, err := util.MessageToStruct(pluginConfig)
					if m.Wasm.Settings != nil {
						isStringSettings := false

						// string类型的配置解析为 google.protobuf.StringValue
						if len(m.Wasm.Settings.Fields) == 1 && m.Wasm.Settings.Fields["_string"] != nil {
							parseTostring := m.Wasm.Settings.Fields["_string"]
							if s, ok := parseTostring.Kind.(*types.Value_StringValue); ok {
								isStringSettings = true
								settings.Fields[util.Struct_Wasm_Configuration] = &types.Value{
									Kind: &types.Value_StructValue{
										StructValue: &types.Struct{
											Fields: map[string]*types.Value{
												util.Struct_Any_AtType: {
													Kind: &types.Value_StringValue{StringValue: util.TypeUrl_StringValue},
												},
												util.Struct_Any_Value: {
													Kind: s,
												},
											},
										},
									},
								}
							}
						}

						// 非string类型的配置解析为 "type.googleapis.com/udpa.type.v1.TypedStruct"
						if !isStringSettings {
							settings.Fields[util.Struct_Wasm_Configuration] = &types.Value{
								Kind: &types.Value_StructValue{
									StructValue: &types.Struct{
										Fields: map[string]*types.Value{
											util.Struct_Any_AtType: {
												Kind: &types.Value_StringValue{StringValue: util.TypeUrl_UdpaTypedStruct},
											},
											util.Struct_Any_Value: {
												Kind: &types.Value_StructValue{StructValue: m.Wasm.Settings},
											},
										},
									},
								},
							}
						}
					}
					if err == nil {
						out.Patch.Value.Fields[util.Struct_HttpFilter_TypedConfig] = &types.Value{
							Kind: &types.Value_StructValue{
								StructValue: &types.Struct{
									Fields: map[string]*types.Value{
										util.Struct_Any_TypedUrl: {
											Kind: &types.Value_StringValue{StringValue: util.TypeUrl_EnvoyFilterHttpWasm},
										},
										util.Struct_Any_AtType: {
											Kind: &types.Value_StringValue{StringValue: util.TypeUrl_UdpaTypedStruct},
										},
										util.Struct_Any_Value: {
											Kind: &types.Value_StructValue{StructValue: &types.Struct{
												Fields: map[string]*types.Value{
													util.Struct_Wasm_Config: {
														Kind: &types.Value_StructValue{
															StructValue: settings,
														},
													},
												},
											}},
										},
									},
								},
							},
						}
					}
				}
			}
		case *microservice.Plugin_Inline:
			out.Patch.Value.Fields[util.Struct_HttpFilter_TypedConfig] = &types.Value{
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{
							util.Struct_Any_TypedUrl: {
								Kind: &types.Value_StringValue{StringValue: in.TypeUrl},
							},
							util.Struct_Any_AtType: {
								Kind: &types.Value_StringValue{StringValue: util.TypeUrl_UdpaTypedStruct},
							},
							util.Struct_Any_Value: {
								Kind: &types.Value_StructValue{StructValue: m.Inline.Settings},
							},
						},
					},
				},
			}
			out.Patch.Value.Fields[util.Struct_HttpFilter_Name] = &types.Value{
				Kind: &types.Value_StringValue{
					StringValue: in.Name,
				},
			}
		}
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
