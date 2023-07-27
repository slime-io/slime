/*
* @Author: yangdihang
* @Date: 2020/6/8
 */

package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"net/url"
	"slime.io/slime/framework/apis/networking/v1alpha3"
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

	applyToDubboVirtualHost   = "DUBBO_SUB_ROUTE_CONFIG"
	applyToDubboRoute         = "DUBBO_ROUTE"
	applyToGenericVirtualHost = "GENERIC_PROXY_VIRTUAL_HOST"
	appToGenericRoute         = "GENERIC_PROXY_ROUTE"
	applyToGenericFilter      = "GENERIC_PROXY_FILTER"
	applyToDubboFilter        = "DUBBO_FILTER"
	applyToHTTPFilter         = "HTTP_FILTER"
)

// genGatewayCfps is a custom func to handle EnvoyPlugin gateway
// default is nil, ignore gateway
var genGatewayCfps func(in *v1alpha1.EnvoyPluginSpec, namespace string, t target, patchCtx istio.EnvoyFilter_PatchContext,
	p *v1alpha1.Plugin, m *v1alpha1.Plugin_Inline) []*istio.EnvoyFilter_EnvoyConfigObjectPatch

type target struct {
	applyToVh   bool
	host, route string
}

var (
	directPatchingPlugins = []string{
		util.EnvoyHTTPRateLimit,
		util.EnvoyHTTPCors,
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
func translatePluginToPatch(name, typeURL string, setting *types.Struct) *istio.EnvoyFilter_Patch {
	return &istio.EnvoyFilter_Patch{Value: translatePluginToPatchValue(name, typeURL, setting)}
}

func valueToTypedStructValue(typeURL string, setting *types.Struct) *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			util.StructAnyValue: {
				Kind: &types.Value_StructValue{StructValue: setting},
			},
			util.StructAnyTypeURL: {
				Kind: &types.Value_StringValue{StringValue: typeURL},
			},
			util.StructAnyAtType: {
				Kind: &types.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
			},
		},
	}
}

func translatePluginToPatchValue(name, typeURL string, setting *types.Struct) *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			util.StructFilterTypedPerFilterConfig: {
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{
							name: {
								Kind: &types.Value_StructValue{StructValue: valueToTypedStructValue(typeURL, setting)},
							},
						},
					},
				},
			},
		},
	}
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

func (r *EnvoyPluginReconciler) translateEnvoyPlugin(cr *v1alpha1.EnvoyPlugin) translateOutput {
	in := cr.Spec.DeepCopy()
	envoyFilter := &istio.EnvoyFilter{}

	if in.WorkloadSelector != nil {
		envoyFilter.WorkloadSelector = &istio.WorkloadSelector{
			Labels: in.WorkloadSelector.Labels,
		}
	}

	envoyFilter.Priority = in.Priority

	var configPatched []translateOutputConfigPatch

	var targets []target
	for _, h := range in.Host {
		targets = append(targets, target{
			applyToVh: true,
			host:      h,
		})
	}
	for _, fullRoute := range in.Route {
		host, route := "", fullRoute
		if ss := strings.SplitN(fullRoute, "/", 2); len(ss) == 2 {
			host, route = ss[0], ss[1]
		}

		targets = append(targets, target{
			host:  host,
			route: route,
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

			if p.Protocol == v1alpha1.Plugin_Dubbo && t.applyToVh {
				// dubbo does not support vh-level filter config
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
			case *v1alpha1.Plugin_Wasm, *v1alpha1.Plugin_Rider:
				log.Errorf("implentment, cause wasm/rider not been support in envoyplugin settings, skip plugin build, plugin: %s")
				continue
			case *v1alpha1.Plugin_Inline:
				if len(in.Gateway) > 0 && genGatewayCfps != nil {
					cfps := genGatewayCfps(in, cr.Namespace, t, patchCtx, p, m)
					for _, cfp := range cfps {
						configPatched = append(configPatched, translateOutputConfigPatch{
							envoyPatch: cfp,
							plugin:     p,
						})
					}
				} else {
					if patchCtx == istio.EnvoyFilter_SIDECAR_OUTBOUND || patchCtx == istio.EnvoyFilter_GATEWAY {
						// ':*' is appended if port info is not specified in outbound and gateway
						// it will match all port in same host after istio adapted
						if len(t.host) > 0 && strings.Index(t.host, ":") == -1 {
							t.host += ":*"
						}
					}

					cfp := generateCfp(t, patchCtx, p, m)
					configPatched = append(configPatched, cfp)
				}
			}
		}
	}
	log.Debugf("translate EnvoyPlugin to Envoyfilter: %v", envoyFilter)

	return translateOutput{
		envoyFilter:   envoyFilter,
		configPatches: configPatched,
	}
}

func generateCfp(t target, patchCtx istio.EnvoyFilter_PatchContext,
	p *v1alpha1.Plugin, m *v1alpha1.Plugin_Inline) translateOutputConfigPatch {
	var (
		extraPatch *types.Struct
		cfp        = &istio.EnvoyFilter_EnvoyConfigObjectPatch{}
		applyTo    string
		match      *types.Struct
	)

	if p.Protocol != v1alpha1.Plugin_HTTP {
		extraPatch = &types.Struct{
			Fields: map[string]*types.Value{},
		}
	}

	switch p.Protocol {
	case v1alpha1.Plugin_HTTP:
		vhMatch := &istio.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{Name: t.host}
		if !t.applyToVh {
			vhMatch.Route = &istio.EnvoyFilter_RouteConfigurationMatch_RouteMatch{Name: t.route}
		}
		cfp.Match = &istio.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: patchCtx,
			ObjectTypes: &istio.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
				RouteConfiguration: &istio.EnvoyFilter_RouteConfigurationMatch{Vhost: vhMatch}},
		}
		if t.applyToVh {
			cfp.ApplyTo = istio.EnvoyFilter_VIRTUAL_HOST
		} else {
			cfp.ApplyTo = istio.EnvoyFilter_HTTP_ROUTE
		}
	case v1alpha1.Plugin_Dubbo:
		// dubbo does not support vh-level filter config
		if t.applyToVh {
			applyTo = applyToDubboVirtualHost
		} else {
			applyTo = applyToDubboRoute
		}
		vhMatch := &types.Struct{Fields: map[string]*types.Value{}}
		// ```yaml
		// dubboRouteConfiguration:
		//   routeConfig:
		//     name: xxx
		//     route:
		//       name: yyy
		// ```
		if t.host != "" {
			addStructField(vhMatch, "name", stringToValue(t.host))
		}
		if !t.applyToVh && t.route != "" {
			addStructField(vhMatch, "route", &types.Value{
				Kind: &types.Value_StructValue{StructValue: fieldToStruct("name", stringToValue(t.route))},
			})
		}
		match = wrapStructField("dubboRouteConfiguration",
			wrapStructField("routeConfig", vhMatch))
	case v1alpha1.Plugin_Generic:
		if t.applyToVh {
			applyTo = applyToGenericVirtualHost
		} else {
			applyTo = appToGenericRoute
		}
		vhMatch := &types.Struct{Fields: map[string]*types.Value{}}
		// ```yaml
		// genericProxyRouteConfiguration:
		//   vhost:
		//     name: xxx
		//     route:
		//       name: yyy
		if t.host != "" {
			addStructField(vhMatch, "name", stringToValue(t.host))
		}
		if !t.applyToVh && t.route != "" {
			addStructField(vhMatch, "route", &types.Value{
				Kind: &types.Value_StructValue{StructValue: fieldToStruct("name", stringToValue(t.route))},
			})
		}
		match = wrapStructField("genericProxyRouteConfiguration",
			wrapStructField("vhost", vhMatch))
	}

	if p.Protocol != v1alpha1.Plugin_HTTP {
		extraPatch.Fields["applyTo"] = &types.Value{Kind: &types.Value_StringValue{StringValue: applyTo}}
		extraPatch.Fields["match"] = &types.Value{Kind: &types.Value_StructValue{StructValue: match}}
	}

	if directPatching(p.Name) {
		cfp.Patch = translateRlsAndCorsToDirectPatch(m.Inline.Settings, !t.applyToVh)
	} else if m.Inline.DirectPatch {
		cfp.Patch = translatePluginToDirectPatch(m.Inline.Settings, m.Inline.FieldPatchTo)
	} else {
		switch p.Protocol {
		case v1alpha1.Plugin_Generic:
			cfp.Patch = translateGenericPluginToPatch(p.Name, p.TypeUrl, m.Inline.Settings)
		case v1alpha1.Plugin_Dubbo:
			fallthrough // same with http
		case v1alpha1.Plugin_HTTP:
			cfp.Patch = translatePluginToPatch(p.Name, p.TypeUrl, m.Inline.Settings)
		}
	}

	cfp.Patch.Operation = istio.EnvoyFilter_Patch_MERGE
	return translateOutputConfigPatch{
		envoyPatch: cfp,
		extraPatch: extraPatch,
		plugin:     p,
	}
}

func addStructField(s *types.Struct, k string, value *types.Value) *types.Struct {
	s.Fields[k] = value
	return s
}

func stringToValue(s string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: s,
		},
	}
}
func fieldToStruct(k string, value *types.Value) *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			k: value,
		},
	}
}
func wrapStructField(k string, value *types.Struct) *types.Struct {
	return fieldToStruct(k, &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: value,
		},
	})
}

func translateGenericPluginToPatch(name string, typeUrl string, settings *types.Struct) *istio.EnvoyFilter_Patch {
	// onMatch:
	//  action:
	//    typedConfig:
	//      @type: typeUrl
	//      perFilterConfig:
	//        envoy.filters.http.lua:
	//          inlineCode: |
	//            function envoy_on_request(request_handle)
	return &istio.EnvoyFilter_Patch{
		Value: wrapStructField("onMatch",
			wrapStructField("action",
				wrapStructField("typedConfig",
					addStructField(
						wrapStructField("perFilterConfig",
							wrapStructField(name, valueToTypedStructValue(typeUrl, settings))),
						"@type", stringToValue(util.TypeURLGenericProxyRouteAction))),
			),
		),
	}
}

type translateOutputConfigPatch struct {
	envoyPatch *istio.EnvoyFilter_EnvoyConfigObjectPatch
	extraPatch *types.Struct
	plugin     *v1alpha1.Plugin
}

type translateOutput struct {
	envoyFilter   *istio.EnvoyFilter
	configPatches []translateOutputConfigPatch
}

func translateOutputToEnvoyFilterWrapper(out translateOutput) (*v1alpha3.EnvoyFilter, error) {
	if out.envoyFilter == nil {
		return nil, nil
	}
	envoyFilterWrapper := &v1alpha3.EnvoyFilter{}

	m, err := util.ProtoToMap(out.envoyFilter)
	if err != nil {
		return nil, err
	}

	if len(out.configPatches) > 0 {
		var appliedPatches []interface{}
		for _, configPatch := range out.configPatches {
			v, err := applyRawPatch(configPatch)
			if err != nil {
				return nil, err
			}
			appliedPatches = append(appliedPatches, v)
		}

		m["configPatches"] = appliedPatches
	}

	envoyFilterWrapper.Spec = m
	return envoyFilterWrapper, nil
}

func applyRawPatch(outputPatch translateOutputConfigPatch) (interface{}, error) {
	m := &gogojsonpb.Marshaler{}
	var buf bytes.Buffer
	if err := m.Marshal(&buf, outputPatch.envoyPatch); err != nil {
		return nil, err
	}
	envoyPatchBytes := buf.Bytes()

	var rawPatches []*types.Struct
	if outputPatch.extraPatch != nil {
		rawPatches = append(rawPatches, outputPatch.extraPatch)
	}
	if rawPatch := outputPatch.plugin.GetRawPatch(); rawPatch != nil {
		rawPatches = append(rawPatches, rawPatch)
	}

	for _, rawPatch := range rawPatches {
		var rawPatchBuf bytes.Buffer
		if err := m.Marshal(&rawPatchBuf, rawPatch); err != nil {
			return nil, err
		}

		bs, err := jsonpatch.MergePatch(envoyPatchBytes, rawPatchBuf.Bytes())
		if err != nil {
			return nil, nil
		}
		envoyPatchBytes = bs
	}

	var ret interface{}
	if err := json.Unmarshal(envoyPatchBytes, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *PluginManagerReconciler) isKnownProtocol(in *v1alpha1.Plugin) bool {
	switch in.Protocol {
	case v1alpha1.Plugin_Generic, v1alpha1.Plugin_Dubbo, v1alpha1.Plugin_HTTP:
		return true
	default:
		return false
	}
}

// translate PluginManager
func (r *PluginManagerReconciler) translatePluginManager(meta metav1.ObjectMeta, in *v1alpha1.PluginManagerSpec) translateOutput {
	var (
		envoyFilter   = &istio.EnvoyFilter{}
		configPatches []translateOutputConfigPatch
	)
	envoyFilter.WorkloadSelector = &istio.WorkloadSelector{
		Labels: in.WorkloadLabels,
	}

	envoyFilter.Priority = in.Priority

	envoyFilter.ConfigPatches = make([]*istio.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	for _, p := range in.Plugin {
		if !p.Enable {
			continue
		}
		if !r.isKnownProtocol(p) {
			continue
		}
		patches, err := r.convertPluginToPatch(meta, p)
		if err != nil {
			log.Errorf("cause error happened, skip plugin build, plugin: %s, %+v", p.Name, err)
			continue
		}

		configPatches = append(configPatches, patches...)
	}

	return translateOutput{
		envoyFilter:   envoyFilter,
		configPatches: configPatches,
	}
}

func (r *PluginManagerReconciler) getListenerFilterName(in *v1alpha1.Plugin) string {
	switch in.Protocol {
	case v1alpha1.Plugin_Generic:
		return util.EnvoyGenericProxy
	case v1alpha1.Plugin_HTTP:
		return util.EnvoyHTTPConnectionManager
	case v1alpha1.Plugin_Dubbo:
		return util.EnvoyDubboProxy
	}
	return ""
}

func (r *PluginManagerReconciler) getSubFilterName(in *v1alpha1.Plugin) string {
	switch in.Protocol {
	case v1alpha1.Plugin_Generic:
		return util.EnvoyGenericProxyRouter
	case v1alpha1.Plugin_HTTP:
		return util.EnvoyHTTPRouter
	case v1alpha1.Plugin_Dubbo:
		return util.EnvoyDubboRouter
	}
	return ""
}

func (r *PluginManagerReconciler) getApplyTo(in *v1alpha1.Plugin) string {
	switch in.Protocol {
	case v1alpha1.Plugin_Generic:
		return applyToGenericFilter
	case v1alpha1.Plugin_Dubbo:
		return applyToDubboFilter
	case v1alpha1.Plugin_HTTP:
		return applyToHTTPFilter
	}
	return ""
}

func (r *PluginManagerReconciler) convertPluginToPatch(meta metav1.ObjectMeta, in *v1alpha1.Plugin) ([]translateOutputConfigPatch, error) {
	listener := &istio.EnvoyFilter_ListenerMatch{
		FilterChain: &istio.EnvoyFilter_ListenerMatch_FilterChainMatch{
			Filter: &istio.EnvoyFilter_ListenerMatch_FilterMatch{
				Name: r.getListenerFilterName(in),
				SubFilter: &istio.EnvoyFilter_ListenerMatch_SubFilterMatch{
					Name: r.getSubFilterName(in),
				},
			},
		},
	}

	if in.Port != 0 {
		listener.PortNumber = in.Port
	}

	defaultApplyTo := istio.EnvoyFilter_HTTP_FILTER
	out := &istio.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: defaultApplyTo,
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
	var extraPatch *types.Struct
	if applyTo := r.getApplyTo(in); applyTo != defaultApplyTo.String() {
		if extraPatch == nil {
			extraPatch = &types.Struct{
				Fields: map[string]*types.Value{},
			}
		}
		extraPatch.Fields["applyTo"] = &types.Value{
			Kind: &types.Value_StringValue{
				StringValue: applyTo,
			},
		}
	}

	ret := []translateOutputConfigPatch{{plugin: in, envoyPatch: out, extraPatch: extraPatch}}

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
		if err := r.applyConfigDiscoveryPlugin(fullResourceName, pluginTypeURL, r.getConfigDiscoveryDefaultConfig(pluginTypeURL), out.Patch.Value); err != nil {
			return err
		}
		filterConfigStruct, err := converter(fullResourceName, meta, in)
		if err != nil {
			return err
		}
		atType, typeURL := "", pluginTypeURL
		// if want raw type, just do: atType, typeURL = typeURL, atType
		return r.addExtensionConfigPath(fullResourceName, toTypedConfig(atType, typeURL, filterConfigStruct), in, &ret)
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

func (r *PluginManagerReconciler) applyConfigDiscoveryPlugin(filterName, typeURL string, defaultConfig *types.Struct, out *types.Struct) error {
	out.Fields[util.StructHttpFilterName] = &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: filterName,
		},
	}

	configDiscoveryFields := map[string]*types.Value{
		util.StructHttpFilterConfigSource: {Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{
			util.StructHttpFilterAds: {Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{}}}},
		}}}},
		util.StructHttpFilterTypeURLs: {Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{
			{Kind: &types.Value_StringValue{StringValue: typeURL}},
		}}}},
	}
	if defaultConfig != nil {
		configDiscoveryFields[util.StructHttpFilterDefaultConfig] = &types.Value{
			Kind: &types.Value_StructValue{StructValue: defaultConfig},
		}
	}
	out.Fields[util.StructHttpFilterConfigDiscovery] = &types.Value{
		Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: configDiscoveryFields}},
	}

	return nil
}

func (r *PluginManagerReconciler) addExtensionConfigPath(filterName string, value *types.Struct, p *v1alpha1.Plugin, target *[]translateOutputConfigPatch) error {
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

	*target = append([]translateOutputConfigPatch{{plugin: p, envoyPatch: out}}, *target...)
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

	pluginSettings := pluginWasm.Wasm.Settings
	if pluginSettings == nil { // use empty struct json string as wasm does not allow nil `configuration`
		pluginSettings = &types.Struct{
			Fields: map[string]*types.Value{
				"_string": {
					Kind: &types.Value_StringValue{
						StringValue: "{}",
					},
				},
			},
		}
	}

	var (
		anyType  string
		anyValue *wrappers.StringValue // != Value_StringValue
	)

	// string类型的配置解析为 google.protobuf.StringValue
	if strField := pluginSettings.Fields["_string"]; strField != nil && len(pluginSettings.Fields) == 1 {
		if _, ok := strField.Kind.(*types.Value_StringValue); ok {
			anyType = util.TypeURLStringValue
			anyValue = &wrappers.StringValue{Value: strField.GetStringValue()}
		}
	}

	// to json string to align with istio behaviour
	if anyValue == nil {
		anyType = util.TypeURLStringValue
		if s, err := (&gogojsonpb.Marshaler{OrigName: true}).MarshalToString(pluginSettings); err != nil {
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
