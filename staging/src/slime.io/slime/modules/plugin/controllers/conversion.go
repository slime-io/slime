/*
* @Author: yangdihang
* @Date: 2020/6/8
 */

package controllers

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	envoyconfigcorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyextensionsfilterswasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/wasm/v3"
	envoyextensionswasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"
	"github.com/gogo/protobuf/types"
	gogojsonpb "github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/protobuf/types/known/durationpb"
	istio "istio.io/api/networking/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"slime.io/slime/framework/util"
	"slime.io/slime/modules/plugin/api/v1alpha1"
)

const (
	fileScheme = "file"
	ociScheme  = "oci"

	// name of environment variable at Wasm VM, which will carry the Wasm image pull secret.
	WasmSecretEnv = "ISTIO_META_WASM_IMAGE_PULL_SECRET"

	nilRemoteCodeSha256 = "nil"

	riderPluginSuffix = ".rider"
	riderPackagePath  = "/usr/local/lib/rider/?/init.lua;/usr/local/lib/rider/?.lua;"
	RiderEnvKey       = "ISTIO_RIDER_ENV"
)

// genGatewayCfps is a custom func to handle EnvoyPlugin gateway
// default is nil, ignore gateway
var genGatewayCfps func(in *v1alpha1.EnvoyPluginSpec, namespace string, t target, patchCtx istio.EnvoyFilter_PatchContext,
	p *v1alpha1.Plugin, m *v1alpha1.Plugin_Inline) []*istio.EnvoyFilter_EnvoyConfigObjectPatch

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

func (r *EnvoyPluginReconciler) translateEnvoyPlugin(cr *v1alpha1.EnvoyPlugin, out *istio.EnvoyFilter) {
	in := cr.Spec.DeepCopy()

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
	p *v1alpha1.Plugin, m *v1alpha1.Plugin_Inline) *istio.EnvoyFilter_EnvoyConfigObjectPatch {
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
func (r *PluginManagerReconciler) translatePluginManager(meta metav1.ObjectMeta, in *v1alpha1.PluginManagerSpec, out *istio.EnvoyFilter) {
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

func (r *PluginManagerReconciler) convertPluginToPatch(meta metav1.ObjectMeta, in *v1alpha1.Plugin) ([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, error) {
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
		resourceName, pluginTypeURL string,
		converter func(name string, meta metav1.ObjectMeta, in *v1alpha1.Plugin) (*types.Struct, error)) error {
		fullResourceName := fmt.Sprintf("%s.%s", meta.Namespace, resourceName)
		if err := r.applyConfigDiscoveryPlugin(fullResourceName, pluginTypeURL, out.Patch.Value); err != nil {
			return err
		}
		filterConfigStruct, err := converter(fullResourceName, meta, in)
		if err != nil {
			return err
		}
		atType, typeURL := "", pluginTypeURL
		// if want raw type, just do: atType, typeURL = typeURL, atType
		return r.addExtensionConfigPath(fullResourceName, toTypedConfig(atType, typeURL, filterConfigStruct), &ret)
	}

	switch m := in.PluginSettings.(type) {
	case *v1alpha1.Plugin_Wasm:
		if err := applyConfigDiscoveryPlugin(in.Name, util.TypeURLEnvoyFilterHTTPWasm, func(resourceName string, meta metav1.ObjectMeta, in *v1alpha1.Plugin) (*types.Struct, error) {
			wasmFilterConfig, err := r.convertWasmFilterConfig(resourceName, meta, in)
			if err != nil {
				return nil, err
			}
			return util.MessageToGogoStruct(wasmFilterConfig)
		}); err != nil {
			return nil, err
		}
	case *v1alpha1.Plugin_Rider:
		if err := applyConfigDiscoveryPlugin(in.Name+riderPluginSuffix, util.TypeURLEnvoyFilterHTTPRider, r.convertRiderFilterConfig); err != nil {
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

func (r *PluginManagerReconciler) applyConfigDiscoveryPlugin(filterName, typeURL string, out *types.Struct) error {
	out.Fields[util.StructHttpFilterName] = &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: filterName,
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

func (r *PluginManagerReconciler) addExtensionConfigPath(filterName string, value *types.Struct, target *[]*istio.EnvoyFilter_EnvoyConfigObjectPatch) error {
	out := &istio.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: istio.EnvoyFilter_EXTENSION_CONFIG,
		Patch: &istio.EnvoyFilter_Patch{
			Operation: istio.EnvoyFilter_Patch_ADD,
			Value: &types.Struct{
				Fields: map[string]*types.Value{
					util.StructHttpFilterName:        {Kind: &types.Value_StringValue{StringValue: filterName}},
					util.StructHttpFilterTypedConfig: {Kind: &types.Value_StructValue{StructValue: value}},
				},
			},
		},
	}

	*target = append([]*istio.EnvoyFilter_EnvoyConfigObjectPatch{out}, *target...)
	return nil
}

func (r *PluginManagerReconciler) convertWasmFilterConfig(resourceName string, meta metav1.ObjectMeta, in *v1alpha1.Plugin) (*envoyextensionsfilterswasmv3.Wasm, error) {
	var (
		wasmEnv    *envoyextensionswasmv3.EnvironmentVariables
		pluginWasm = in.PluginSettings.(*v1alpha1.Plugin_Wasm)
	)

	datasource, err := convertDataSource(pluginWasm.Wasm.Url, pluginWasm.Wasm.Sha256)
	if err != nil {
		return nil, err
	}

	if datasource.GetRemote() != nil {
		imagePullSecretContent, err := r.convertImagePullSecret(pluginWasm.Wasm.GetImagePullSecretName(), pluginWasm.Wasm.GetImagePullSecretContent(), meta.Namespace)
		if err != nil {
			return nil, err
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
		Name:   resourceName,
		RootId: pluginWasm.Wasm.PluginName,
		Vm: &envoyextensionswasmv3.PluginConfig_VmConfig{
			VmConfig: &envoyextensionswasmv3.VmConfig{
				Runtime:              util.EnvoyWasmV8,
				Code:                 datasource,
				EnvironmentVariables: wasmEnv,
			},
		},
	}

	if settings := pluginWasm.Wasm.Settings; settings != nil {
		var (
			anyType  string
			anyValue *wrappers.StringValue // != Value_StringValue
		)

		// string类型的配置解析为 google.protobuf.StringValue
		if strField := settings.Fields["_string"]; strField != nil && len(settings.Fields) == 1 {
			if _, ok := strField.Kind.(*types.Value_StringValue); ok {
				anyType = util.TypeURLStringValue
				anyValue = &wrappers.StringValue{Value: strField.GetStringValue()}
			}
		}

		// to json string to align with istio behaviour
		if anyValue == nil {
			anyType = util.TypeURLStringValue
			if s, err := (&gogojsonpb.Marshaler{OrigName: true}).MarshalToString(settings); err != nil {
				return nil, err
			} else {
				anyValue = &wrappers.StringValue{Value: s}
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

func (r *PluginManagerReconciler) convertRiderFilterConfig(resourceName string, meta metav1.ObjectMeta, in *v1alpha1.Plugin) (*types.Struct, error) {
	var (
		pluginRider            = in.PluginSettings.(*v1alpha1.Plugin_Rider)
		imagePullSecretContent string
		err                    error
	)

	datasource, err := convertDataSource(pluginRider.Rider.Url, pluginRider.Rider.Sha256)
	if err != nil {
		return nil, err
	}

	if datasource.GetRemote() != nil {
		if imagePullSecretContent, err = r.convertImagePullSecret(
			pluginRider.Rider.GetImagePullSecretName(), pluginRider.Rider.GetImagePullSecretContent(),
			meta.Namespace); err != nil {
			return nil, err
		}
	}

	datasourceStruct, err := util.MessageToGogoStruct(datasource)
	if err != nil {
		return nil, err
	}

	riderPluginConfig := &types.Struct{Fields: map[string]*types.Value{
		"name": {Kind: &types.Value_StringValue{StringValue: pluginRider.Rider.PluginName}},
		"vm_config": {Kind: &types.Value_StructValue{StructValue: &types.Struct{
			Fields: map[string]*types.Value{
				"package_path": {Kind: &types.Value_StringValue{StringValue: riderPackagePath}},
			},
		}}},
		"code": {Kind: &types.Value_StructValue{StructValue: datasourceStruct}},
	}}
	riderFilterConfig := &types.Struct{
		Fields: map[string]*types.Value{
			"plugin": {Kind: &types.Value_StructValue{StructValue: riderPluginConfig}},
		},
	}

	config := pluginRider.Rider.Settings
	ensureEnv := func() *types.Struct {
		if config.GetFields() == nil {
			config = &types.Struct{Fields: map[string]*types.Value{}}
		}

		envSt := config.Fields[RiderEnvKey].GetStructValue()
		if envSt == nil {
			envSt = &types.Struct{Fields: map[string]*types.Value{}}
			config.Fields[RiderEnvKey] = &types.Value{Kind: &types.Value_StructValue{StructValue: envSt}}
		}
		if envSt.Fields == nil {
			envSt.Fields = map[string]*types.Value{}
		}
		return envSt
	}
	if imagePullSecretContent != "" {
		ensureEnv().Fields[WasmSecretEnv] = &types.Value{Kind: &types.Value_StringValue{StringValue: imagePullSecretContent}}
	}

	if config != nil {
		riderPluginConfig.Fields["config"] = &types.Value{Kind: &types.Value_StructValue{StructValue: config}}
	}

	return riderFilterConfig, nil
}

func convertDataSource(urlStr, sha256 string) (*envoyconfigcorev3.AsyncDataSource, error) {
	var (
		imageURL   *url.URL
		datasource *envoyconfigcorev3.AsyncDataSource
	)
	if v, err := url.Parse(urlStr); err != nil {
		return nil, err
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
						Filename: strings.TrimPrefix(urlStr, "file://"),
					},
				},
			},
		}
	} else {
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
	}

	return datasource, nil
}

func (r *PluginManagerReconciler) convertImagePullSecret(name, content, ns string) (string, error) {
	if content != "" {
		return content, nil
	}
	if name == "" {
		return "", nil
	}

	if r.credController == nil {
		return "", fmt.Errorf("plugin use secret %s but cred controller disabled", name)
	}
	secretBytes, err := r.credController.GetDockerCredential(name, ns)
	if err != nil {
		return "", fmt.Errorf("plugin: use secret %s but get secret met err %+v", name, err)
	}
	return string(secretBytes), nil
}
