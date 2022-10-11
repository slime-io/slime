/*
* @Author: yangdihang
* @Date: 2020/6/8
 */

package controllers

import (
	"fmt"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/url"
	"strings"
	"time"

	"slime.io/slime/framework/util"
	"slime.io/slime/modules/plugin/api/v1alpha1"
	microserviceslimeiov1alpha1types "slime.io/slime/modules/plugin/api/v1alpha1"
	microserviceslimeiov1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1/wrapper"

	envoyconfigcorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyextensionsfilterswasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/wasm/v3"
	envoyextensionswasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"github.com/gogo/protobuf/types"
	istio "istio.io/api/networking/v1alpha3"
)

const (
	fileScheme = "file"
	ociScheme  = "oci"

	// name of environment variable at Wasm VM, which will carry the Wasm image pull secret.
	WasmSecretEnv = "ISTIO_META_WASM_IMAGE_PULL_SECRET"

	nilRemoteCodeSha256 = "nil"
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
										util.StructAnyTypeURL: {
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
func (r *PluginManagerReconciler) translatePluginManager(meta metav1.ObjectMeta, in *microserviceslimeiov1alpha1types.PluginManager, out *istio.EnvoyFilter) {
	out.WorkloadSelector = &istio.WorkloadSelector{
		Labels: in.WorkloadLabels,
	}
	out.ConfigPatches = make([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	for _, p := range in.Plugin {
		if !p.Enable {
			continue
		}
		patches, err := r.convertPluginToPatch(meta, p)
		if err != nil {
			log.Errorf("cause error happened, skip plugin build, plugin: %s, %+v", p.Name, err)
			continue
		}

		out.ConfigPatches = append(out.ConfigPatches, patches...)
	}
}

func (r *PluginManagerReconciler) convertPluginToPatch(meta metav1.ObjectMeta, in *microserviceslimeiov1alpha1types.Plugin) ([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, error) {
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
		if err := r.applyInlinePlugin(in.Name, in.TypeUrl, nil, out.Patch.Value); err != nil {
			return nil, err
		}
		return ret, nil
	}

	applyConfigDiscoveryPlugin := func(
		pluginTypeURL string,
		converter func(name string, meta metav1.ObjectMeta, in *microserviceslimeiov1alpha1types.Plugin) (*types.Struct, error)) error {
		name := fmt.Sprintf("%s.%s", meta.Namespace, in.Name)
		if err := r.applyConfigDiscoveryPlugin(name, pluginTypeURL, out.Patch.Value); err != nil {
			return err
		}
		filterConfigStruct, err := converter(name, meta, in)
		if err != nil {
			return err
		}
		atType, typeURL := "", pluginTypeURL
		// if want raw type, just do: atType, typeURL = typeURL, atType
		return r.addExtensionConfigPath(name, toTypedConfig(atType, typeURL, filterConfigStruct), &ret)
	}

	switch m := in.PluginSettings.(type) {
	case *v1alpha1.Plugin_Wasm:
		if err := applyConfigDiscoveryPlugin(util.TypeURLEnvoyFilterHTTPWasm, func(name string, meta metav1.ObjectMeta, in *microserviceslimeiov1alpha1types.Plugin) (*types.Struct, error) {
			wasmFilterConfig, err := r.convertWasmFilterConfig(name, meta, in)
			if err != nil {
				return nil, err
			}
			return util.MessageToGogoStruct(wasmFilterConfig)
		}); err != nil {
			return nil, err
		}
	case *v1alpha1.Plugin_Rider:
		if err := applyConfigDiscoveryPlugin(util.TypeURLEnvoyFilterHTTPRider, r.convertRiderFilterConfig); err != nil {
			return nil, err
		}
	case *v1alpha1.Plugin_Inline:
		if err := r.applyInlinePlugin(in.Name, in.TypeUrl, m, out.Patch.Value); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func toTypedConfig(atType, typeURL string, value *types.Struct) *types.Struct {
	if typeURL != "" {
		return util.ToTypedStruct(typeURL, value)
	}
	value.Fields[util.StructAnyAtType] = &types.Value{Kind: &types.Value_StringValue{StringValue: atType}}
	return value
}

func (r *PluginManagerReconciler) applyInlinePlugin(name, typeURL string, settings *v1alpha1.Plugin_Inline, out *types.Struct) error {
	out.Fields[util.StructHttpFilterName] = &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: name,
		},
	}

	if settings != nil {
		out.Fields[util.StructHttpFilterTypedConfig] = &types.Value{
			Kind: &types.Value_StructValue{
				StructValue: &types.Struct{
					Fields: map[string]*types.Value{
						util.StructAnyTypeURL: {
							Kind: &types.Value_StringValue{StringValue: typeURL},
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

func (r *PluginManagerReconciler) applyConfigDiscoveryPlugin(name, typeURL string, out *types.Struct) error {
	out.Fields[util.StructHttpFilterName] = &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: name,
		},
	}
	out.Fields[util.StructHttpFilterConfigDiscovery] = &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{Fields: map[string]*types.Value{
				util.StructHttpFilterConfigSource: {Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{
					util.StructHttpFilterAds: {Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{}}}},
				}}}},
				util.StructHttpFilterTypeURLs: {Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{
					{Kind: &types.Value_StringValue{StringValue: typeURL}},
				}}}},
			}},
		},
	}

	return nil
}

func (r *PluginManagerReconciler) addExtensionConfigPath(name string, value *types.Struct, target *[]*istio.EnvoyFilter_EnvoyConfigObjectPatch) error {
	out := &istio.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: istio.EnvoyFilter_EXTENSION_CONFIG,
		Patch: &istio.EnvoyFilter_Patch{
			Operation: istio.EnvoyFilter_Patch_ADD,
			Value: &types.Struct{
				Fields: map[string]*types.Value{
					util.StructHttpFilterName:        {Kind: &types.Value_StringValue{StringValue: name}},
					util.StructHttpFilterTypedConfig: {Kind: &types.Value_StructValue{StructValue: value}},
				},
			},
		},
	}

	*target = append([]*istio.EnvoyFilter_EnvoyConfigObjectPatch{out}, *target...)
	return nil
}

func (r *PluginManagerReconciler) convertWasmFilterConfig(name string, meta metav1.ObjectMeta, in *microserviceslimeiov1alpha1types.Plugin) (*envoyextensionsfilterswasmv3.Wasm, error) {
	var (
		imageURL   *url.URL
		datasource *envoyconfigcorev3.AsyncDataSource
		wasmEnv    *envoyextensionswasmv3.EnvironmentVariables
		pluginWasm = in.PluginSettings.(*microserviceslimeiov1alpha1types.Plugin_Wasm)
	)
	if v, err := url.Parse(pluginWasm.Wasm.Url); err != nil {
		return nil, fmt.Errorf("plugin:%s, invalid url %s", in.Name, pluginWasm.Wasm.Url)
	} else {
		imageURL = v
	}

	// when no scheme is given, default to oci scheme
	if imageURL.Scheme == "" {
		imageURL.Scheme = ociScheme
	}

	if imageURL.Scheme == fileScheme {
		datasource = &envoyconfigcorev3.AsyncDataSource{
			Specifier: &envoyconfigcorev3.AsyncDataSource_Local{
				Local: &envoyconfigcorev3.DataSource{
					Specifier: &envoyconfigcorev3.DataSource_Filename{
						Filename: strings.TrimPrefix(pluginWasm.Wasm.Url, "file://"),
					},
				},
			},
		}
	} else {
		sha256 := pluginWasm.Wasm.Sha256
		if sha256 == "" {
			sha256 = nilRemoteCodeSha256
		}
		datasource = &envoyconfigcorev3.AsyncDataSource{
			Specifier: &envoyconfigcorev3.AsyncDataSource_Remote{
				Remote: &envoyconfigcorev3.RemoteDataSource{
					HttpUri: &envoyconfigcorev3.HttpUri{
						Uri:     imageURL.String(),
						Timeout: durationpb.New(30 * time.Second),
						HttpUpstreamType: &envoyconfigcorev3.HttpUri_Cluster{
							// this will be fetched by the agent anyway, so no need for a cluster
							Cluster: "_",
						},
					},
					Sha256: sha256,
				},
			},
		}

		var imagePullSecretContent string
		if imagePullSecretContent = pluginWasm.Wasm.GetImagePullSecretContent(); imagePullSecretContent == "" {
			if secretName := pluginWasm.Wasm.GetImagePullSecretName(); secretName != "" {
				if r.credController == nil {
					return nil, fmt.Errorf("plugin:%s use secret %s but cred controller disabled", in.Name, secretName)
				}
				secretBytes, err := r.credController.GetDockerCredential(secretName, meta.Namespace)
				if err != nil {
					return nil, fmt.Errorf("plugin:%s use secret %s but get secret met err %+v", in.Name, secretName, err)
				}
				imagePullSecretContent = string(secretBytes)
			}
		}

		if imagePullSecretContent != "" {
			wasmEnv = &envoyextensionswasmv3.EnvironmentVariables{
				KeyValues: map[string]string{
					WasmSecretEnv: imagePullSecretContent,
				},
			}
		}
	}

	pluginConfig := &envoyextensionswasmv3.PluginConfig{
		Name:   name,
		RootId: pluginWasm.Wasm.PluginName,
		Vm: &envoyextensionswasmv3.PluginConfig_VmConfig{
			VmConfig: &envoyextensionswasmv3.VmConfig{
				Runtime:              util.EnvoyWasmV8,
				Code:                 datasource,
				EnvironmentVariables: wasmEnv,
			},
		},
	}

	if pluginWasm.Wasm.Settings != nil {
		var (
			anyType  string
			anyValue *types.Value
		)

		// string类型的配置解析为 google.protobuf.StringValue
		if len(pluginWasm.Wasm.Settings.Fields) == 1 && pluginWasm.Wasm.Settings.Fields["_string"] != nil {
			parseTostring := pluginWasm.Wasm.Settings.Fields["_string"]
			if s, ok := parseTostring.Kind.(*types.Value_StringValue); ok {
				anyType = util.TypeURLStringValue
				anyValue = &types.Value{Kind: s}
			}
		}

		// to json string to align with istio behaviour
		if anyValue == nil {
			anyType = util.TypeURLStringValue
			if s, err := (&jsonpb.Marshaler{OrigName: true}).MarshalToString(pluginWasm.Wasm.Settings); err != nil {
				return nil, err
			} else {
				anyValue = &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
			}
		}

		valueBytes, err := proto.Marshal(anyValue)
		if err != nil {
			return nil, err
		}

		pluginConfig.Configuration = &any.Any{
			TypeUrl: anyType,
			Value:   valueBytes,
		}
	}

	return &envoyextensionsfilterswasmv3.Wasm{Config: pluginConfig}, nil
}

func (r *PluginManagerReconciler) convertRiderFilterConfig(name string, meta metav1.ObjectMeta, in *microserviceslimeiov1alpha1types.Plugin) (*types.Struct, error) {
	// TODO
	return nil, nil
}
