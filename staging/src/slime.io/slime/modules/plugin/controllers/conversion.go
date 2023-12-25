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
	jsonpatch "github.com/evanphx/json-patch/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	any "google.golang.org/protobuf/types/known/anypb"
	duration "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	networkingapi "istio.io/api/networking/v1alpha3"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
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

// genGatewayInlineCfps is a custom func to handle EnvoyPlugin gateway
// default is nil, ignore gateway
var genGatewayInlineCfps func(in *v1alpha1.EnvoyPluginSpec, namespace string, t target, patchCtx networkingapi.EnvoyFilter_PatchContext,
	p *v1alpha1.Plugin, m *v1alpha1.Inline) []*networkingapi.EnvoyFilter_EnvoyConfigObjectPatch

type target struct {
	applyToVh   bool
	host, route string
}

var directPatchingPlugins = []string{
	util.EnvoyHTTPRateLimit,
	util.EnvoyHTTPCors,
	util.EnvoyRatelimitV1, // keep backward compatibility
}

func directPatching(name string) bool {
	for _, plugin := range directPatchingPlugins {
		if name == plugin {
			return true
		}
	}
	return false
}

// translate EnvoyPlugin
func translatePluginToPatch(name, typeURL string, setting *structpb.Struct) *networkingapi.EnvoyFilter_Patch {
	return &networkingapi.EnvoyFilter_Patch{Value: translatePluginToPatchValue(name, typeURL, setting)}
}

func stringValueToStruct(strValue *wrappers.StringValue) *structpb.Struct {
	if strValue == nil {
		return nil
	}
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			util.StructAnyAtType: {
				Kind: &structpb.Value_StringValue{StringValue: util.TypeURLStringValue},
			},
			util.StructAnyValue: {
				Kind: &structpb.Value_StringValue{StringValue: strValue.Value},
			},
		},
	}
}

func valueToTypedStructValue(typeURL string, setting *structpb.Struct) *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			util.StructAnyValue: {
				Kind: &structpb.Value_StructValue{StructValue: setting},
			},
			util.StructAnyTypeURL: {
				Kind: &structpb.Value_StringValue{StringValue: typeURL},
			},
			util.StructAnyAtType: {
				Kind: &structpb.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
			},
		},
	}
}

func translatePluginToPatchValue(name, typeURL string, setting *structpb.Struct) *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			util.StructFilterTypedPerFilterConfig: {
				Kind: &structpb.Value_StructValue{
					StructValue: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							name: {
								Kind: &structpb.Value_StructValue{StructValue: valueToTypedStructValue(typeURL, setting)},
							},
						},
					},
				},
			},
		},
	}
}

func translateRlsAndCorsToDirectPatch(settings *structpb.Struct, applyToHTTPRoute bool) *networkingapi.EnvoyFilter_Patch {
	fieldPatchTo := ""
	if applyToHTTPRoute {
		fieldPatchTo = "route"
	}
	return translatePluginToDirectPatch(settings, fieldPatchTo)
}

func translatePluginToDirectPatch(settings *structpb.Struct, fieldPatchTo string) *networkingapi.EnvoyFilter_Patch {
	patch := &networkingapi.EnvoyFilter_Patch{}

	if fieldPatchTo == "ROOT" {
		fieldPatchTo = ""
	}

	if fieldPatchTo != "" {
		patch.Value = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				fieldPatchTo: {
					Kind: &structpb.Value_StructValue{
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
	envoyFilter := &networkingapi.EnvoyFilter{}
	proxyVersion := r.Cfg.GetProxyVersion()

	if in.WorkloadSelector != nil {
		envoyFilter.WorkloadSelector = &networkingapi.WorkloadSelector{
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

			patchCtx := networkingapi.EnvoyFilter_ANY
			if !strings.HasPrefix(t.host, "inbound|") { // keep backward compatibility
				switch p.ListenerType {
				case v1alpha1.Plugin_Outbound:
					patchCtx = networkingapi.EnvoyFilter_SIDECAR_OUTBOUND
				case v1alpha1.Plugin_Inbound:
					patchCtx = networkingapi.EnvoyFilter_SIDECAR_INBOUND
				case v1alpha1.Plugin_Gateway:
					patchCtx = networkingapi.EnvoyFilter_GATEWAY
				}
			}

			var (
				inline      *v1alpha1.Inline
				pluginInUse = *p
			)
			switch pluginSettings := p.PluginSettings.(type) {
			case *v1alpha1.Plugin_Wasm:
				strValue, err := convertWasmConfigurationToStringValue(pluginSettings.Wasm.Settings)
				if err != nil {
					log.Errorf("convert wasm configuration to string value failed, skip plugin build, plugin: %s", p.Name)
					continue
				}
				// ```yaml
				// configuration:
				//   '@type': type.googleapis.com/google.protobuf.StringValue
				//   value: '{}'
				// ```
				inline = &v1alpha1.Inline{
					Settings: wrapStructToStruct("configuration", stringValueToStruct(strValue)),
				}
				pluginInUse.Name = getConfigDiscoveryFilterFullName(cr.Namespace, p.Name)
			case *v1alpha1.Plugin_Rider:
				// ```yaml
				// plugins:
				// - config: {}
				//   name: xx
				// ```
				inline = &v1alpha1.Inline{
					Settings: fieldToStruct("plugins",
						wrapStructToStructWrapper("config", pluginSettings.Rider.Settings).
							AddStringField("name", p.Name).WrapToListValue(),
					),
				}
				pluginInUse.Name = getConfigDiscoveryFilterFullName(cr.Namespace, getFullRiderPluginName(p.Name))
			case *v1alpha1.Plugin_Inline:
				inline = pluginSettings.Inline
			default:
				log.Errorf("unknown plugin settings type, skip plugin build, plugin: %s", p.Name)
				continue
			}

			if len(in.Gateway) > 0 && genGatewayInlineCfps != nil {
				cfps := genGatewayInlineCfps(in, cr.Namespace, t, patchCtx, &pluginInUse, inline)
				for _, cfp := range cfps {
					configPatched = append(configPatched, translateOutputConfigPatch{
						envoyPatch: cfp,
						plugin:     &pluginInUse,
					})
				}
			} else {
				if patchCtx == networkingapi.EnvoyFilter_SIDECAR_OUTBOUND || patchCtx == networkingapi.EnvoyFilter_GATEWAY {
					// ':*' is appended if port info is not specified in outbound and gateway
					// it will match all port in same host after istio adapted
					if len(t.host) > 0 && strings.Index(t.host, ":") == -1 {
						t.host += ":*"
					}
				}

				cfp := generateInlineCfp(t, patchCtx, &pluginInUse, inline, proxyVersion)
				configPatched = append(configPatched, cfp)
			}
		}
	}
	log.Debugf("translate EnvoyPlugin to Envoyfilter: %v", envoyFilter)

	return translateOutput{
		envoyFilter:   envoyFilter,
		configPatches: configPatched,
	}
}

func generateInlineCfp(t target, patchCtx networkingapi.EnvoyFilter_PatchContext,
	p *v1alpha1.Plugin, inline *v1alpha1.Inline, proxyVersion string,
) translateOutputConfigPatch {
	var (
		extraPatch *structpb.Struct
		cfp        = &networkingapi.EnvoyFilter_EnvoyConfigObjectPatch{}
		applyTo    string
		match      *structpb.Struct
	)

	if p.Protocol != v1alpha1.Plugin_HTTP {
		extraPatch = &structpb.Struct{
			Fields: map[string]*structpb.Value{},
		}
	}

	switch p.Protocol {
	case v1alpha1.Plugin_HTTP:
		vhMatch := &networkingapi.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{Name: t.host}
		if !t.applyToVh {
			vhMatch.Route = &networkingapi.EnvoyFilter_RouteConfigurationMatch_RouteMatch{Name: t.route}
		}
		cfp.Match = &networkingapi.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: patchCtx,
			ObjectTypes: &networkingapi.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
				RouteConfiguration: &networkingapi.EnvoyFilter_RouteConfigurationMatch{Vhost: vhMatch},
			},
		}

		// only apply to protocol http
		// dubbo or other support will be considered later
		if proxyVersion != "" {
			cfp.Match.Proxy = &networkingapi.EnvoyFilter_ProxyMatch{
				ProxyVersion: proxyVersion,
			}
		}
		if t.applyToVh {
			cfp.ApplyTo = networkingapi.EnvoyFilter_VIRTUAL_HOST
		} else {
			cfp.ApplyTo = networkingapi.EnvoyFilter_HTTP_ROUTE
		}
	case v1alpha1.Plugin_Dubbo:
		// dubbo does not support vh-level filter config
		if t.applyToVh {
			applyTo = applyToDubboVirtualHost
		} else {
			applyTo = applyToDubboRoute
		}
		vhMatch := &structpb.Struct{Fields: map[string]*structpb.Value{}}
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
			addStructField(vhMatch, "route", &structpb.Value{
				Kind: &structpb.Value_StructValue{StructValue: fieldToStruct("name", stringToValue(t.route))},
			})
		}
		match = wrapStructToStruct("dubboRouteConfiguration",
			wrapStructToStruct("routeConfig", vhMatch))
	case v1alpha1.Plugin_Generic:
		if t.applyToVh {
			applyTo = applyToGenericVirtualHost
		} else {
			applyTo = appToGenericRoute
		}
		vhMatch := &structpb.Struct{Fields: map[string]*structpb.Value{}}
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
			addStructField(vhMatch, "route", &structpb.Value{
				Kind: &structpb.Value_StructValue{StructValue: fieldToStruct("name", stringToValue(t.route))},
			})
		}
		match = wrapStructToStruct("genericProxyRouteConfiguration",
			wrapStructToStruct("vhost", vhMatch))
	}

	if p.Protocol != v1alpha1.Plugin_HTTP {
		extraPatch.Fields["applyTo"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: applyTo}}
		extraPatch.Fields["match"] = &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: match}}
	}

	if directPatching(p.Name) {
		cfp.Patch = translateRlsAndCorsToDirectPatch(inline.Settings, !t.applyToVh)
	} else if inline.DirectPatch {
		cfp.Patch = translatePluginToDirectPatch(inline.Settings, inline.FieldPatchTo)
	} else {
		switch p.Protocol {
		case v1alpha1.Plugin_Generic:
			cfp.Patch = translateGenericPluginToPatch(p.Name, p.TypeUrl, inline.Settings)
		case v1alpha1.Plugin_Dubbo:
			fallthrough // same with http
		case v1alpha1.Plugin_HTTP:
			cfp.Patch = translatePluginToPatch(p.Name, p.TypeUrl, inline.Settings)
		}
	}

	cfp.Patch.Operation = networkingapi.EnvoyFilter_Patch_MERGE
	return translateOutputConfigPatch{
		envoyPatch: cfp,
		extraPatch: extraPatch,
		plugin:     p,
	}
}

func translateGenericPluginToPatch(name string, typeUrl string, settings *structpb.Struct) *networkingapi.EnvoyFilter_Patch {
	// onMatch:
	//  action:
	//    typedConfig:
	//      @type: typeUrl
	//      perFilterConfig:
	//        envoy.filters.http.lua:
	//          inlineCode: |
	//            function envoy_on_request(request_handle)
	return &networkingapi.EnvoyFilter_Patch{
		Value: wrapStructToStruct(
			"onMatch", wrapStructToStruct(
				"action", wrapStructToStruct(
					"typedConfig", wrapStructToStructWrapper(
						"perFilterConfig", wrapStructToStruct(name, valueToTypedStructValue(typeUrl, settings))).AddStringField(
						"@type", util.TypeURLGenericProxyRouteAction).Get(),
				),
			)),
	}
}

type translateOutputConfigPatch struct {
	envoyPatch *networkingapi.EnvoyFilter_EnvoyConfigObjectPatch
	extraPatch *structpb.Struct
	plugin     *v1alpha1.Plugin
}

type translateOutput struct {
	envoyFilter   *networkingapi.EnvoyFilter
	configPatches []translateOutputConfigPatch
}

func translateOutputToEnvoyFilterWrapper(out translateOutput) (*networkingv1alpha3.EnvoyFilter, error) {
	if out.envoyFilter == nil {
		return nil, nil
	}
	envoyFilterWrapper := &networkingv1alpha3.EnvoyFilter{}
	envoyFilterWrapper.Spec = *out.envoyFilter

	if len(out.configPatches) > 0 {
		var appliedPatches []*networkingapi.EnvoyFilter_EnvoyConfigObjectPatch
		for _, configPatch := range out.configPatches {
			v, err := applyRawPatch(configPatch)
			if err != nil {
				return nil, err
			}
			appliedPatches = append(appliedPatches, v)
		}
		envoyFilterWrapper.Spec.ConfigPatches = appliedPatches
	}
	return envoyFilterWrapper, nil
}

func applyRawPatch(outputPatch translateOutputConfigPatch) (*networkingapi.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	envoyPatchBytes, err := protojson.Marshal(outputPatch.envoyPatch)
	if err != nil {
		return nil, err
	}

	var rawPatches []*structpb.Struct
	if outputPatch.extraPatch != nil {
		rawPatches = append(rawPatches, outputPatch.extraPatch)
	}
	if rawPatch := outputPatch.plugin.GetRawPatch(); rawPatch != nil {
		rawPatches = append(rawPatches, rawPatch)
	}

	for _, rawPatch := range rawPatches {
		rawPatchBytes, err := protojson.Marshal(rawPatch)
		if err != nil {
			return nil, err
		}

		bs, err := jsonpatch.MergePatch(envoyPatchBytes, rawPatchBytes)
		if err != nil {
			return nil, nil
		}
		envoyPatchBytes = bs
	}

	var ret networkingapi.EnvoyFilter_EnvoyConfigObjectPatch
	if err := protojson.Unmarshal(envoyPatchBytes, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
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
		envoyFilter   = &networkingapi.EnvoyFilter{}
		configPatches []translateOutputConfigPatch
	)
	envoyFilter.WorkloadSelector = &networkingapi.WorkloadSelector{
		Labels: in.WorkloadLabels,
	}

	envoyFilter.Priority = in.Priority

	envoyFilter.ConfigPatches = make([]*networkingapi.EnvoyFilter_EnvoyConfigObjectPatch, 0)
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

func getFullRiderPluginName(name string) string {
	return name + riderPluginSuffix
}

func getConfigDiscoveryFilterFullName(ns, name string) string {
	return fmt.Sprintf("%s.%s", ns, name)
}

func (r *PluginManagerReconciler) convertPluginToPatch(meta metav1.ObjectMeta, in *v1alpha1.Plugin) ([]translateOutputConfigPatch, error) {
	listener := &networkingapi.EnvoyFilter_ListenerMatch{
		FilterChain: &networkingapi.EnvoyFilter_ListenerMatch_FilterChainMatch{
			Filter: &networkingapi.EnvoyFilter_ListenerMatch_FilterMatch{
				Name: r.getListenerFilterName(in),
				SubFilter: &networkingapi.EnvoyFilter_ListenerMatch_SubFilterMatch{
					Name: r.getSubFilterName(in),
				},
			},
		},
	}

	if in.Port != 0 {
		listener.PortNumber = in.Port
	}

	defaultApplyTo := networkingapi.EnvoyFilter_HTTP_FILTER
	out := &networkingapi.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: defaultApplyTo,
		Match: &networkingapi.EnvoyFilter_EnvoyConfigObjectMatch{
			ObjectTypes: &networkingapi.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
				Listener: listener,
			},
		},
		Patch: &networkingapi.EnvoyFilter_Patch{
			Operation: networkingapi.EnvoyFilter_Patch_INSERT_BEFORE,
			Value: &structpb.Struct{
				Fields: map[string]*structpb.Value{},
			},
		},
	}

	// if proxyVersion is set, apply specified proxyVersion
	if proxyVersion := r.cfg.GetProxyVersion(); proxyVersion != "" {
		out.Match.Proxy = &networkingapi.EnvoyFilter_ProxyMatch{
			ProxyVersion: proxyVersion,
		}
	}

	if in.DisableOnFilterLevel {
		out.Patch.Value.Fields[util.StructFilterDisabled] = &structpb.Value{
			Kind: &structpb.Value_BoolValue{BoolValue: true},
		}
	}

	var extraPatch *structpb.Struct
	if applyTo := r.getApplyTo(in); applyTo != defaultApplyTo.String() {
		if extraPatch == nil {
			extraPatch = &structpb.Struct{
				Fields: map[string]*structpb.Value{},
			}
		}
		extraPatch.Fields["applyTo"] = &structpb.Value{
			Kind: &structpb.Value_StringValue{
				StringValue: applyTo,
			},
		}
	}

	ret := []translateOutputConfigPatch{{plugin: in, envoyPatch: out, extraPatch: extraPatch}}

	switch in.ListenerType {
	case v1alpha1.Plugin_Outbound:
		out.Match.Context = networkingapi.EnvoyFilter_SIDECAR_OUTBOUND
	case v1alpha1.Plugin_Inbound:
		out.Match.Context = networkingapi.EnvoyFilter_SIDECAR_INBOUND
	case v1alpha1.Plugin_Gateway:
		out.Match.Context = networkingapi.EnvoyFilter_GATEWAY
	}

	if in.PluginSettings == nil {
		if err := r.applyInlinePlugin(in.Name, in.TypeUrl, nil, out.Patch.Value); err != nil {
			return nil, err
		}
		return ret, nil
	}

	applyConfigDiscoveryPlugin := func(
		resourceName, pluginTypeURL string,
		converter func(name string, meta metav1.ObjectMeta, in *v1alpha1.Plugin) (*structpb.Struct, error),
	) error {
		fullResourceName := getConfigDiscoveryFilterFullName(meta.Namespace, resourceName)
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
		if err := applyConfigDiscoveryPlugin(in.Name, util.TypeURLEnvoyFilterHTTPWasm, func(resourceName string, meta metav1.ObjectMeta, in *v1alpha1.Plugin) (*structpb.Struct, error) {
			wasmFilterConfig, err := r.convertWasmFilterConfig(resourceName, meta, in)
			if err != nil {
				return nil, err
			}
			return util.MessageToStruct(wasmFilterConfig)
		}); err != nil {
			return nil, err
		}
	case *v1alpha1.Plugin_Rider:
		if err := applyConfigDiscoveryPlugin(getFullRiderPluginName(in.Name), util.TypeURLEnvoyFilterHTTPRider, r.convertRiderFilterConfig); err != nil {
			return nil, err
		}
	case *v1alpha1.Plugin_Inline:
		if err := r.applyInlinePlugin(in.Name, in.TypeUrl, m, out.Patch.Value); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func toTypedConfig(atType, typeURL string, value *structpb.Struct) *structpb.Struct {
	if typeURL != "" {
		return util.ToTypedStruct(typeURL, value)
	}
	value.Fields[util.StructAnyAtType] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: atType}}
	return value
}

func (r *PluginManagerReconciler) applyInlinePlugin(name, typeURL string, settings *v1alpha1.Plugin_Inline, out *structpb.Struct) error {
	out.Fields[util.StructHttpFilterName] = &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: name,
		},
	}

	if settings != nil {
		out.Fields[util.StructHttpFilterTypedConfig] = &structpb.Value{
			Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						util.StructAnyTypeURL: {
							Kind: &structpb.Value_StringValue{StringValue: typeURL},
						},
						util.StructAnyAtType: {
							Kind: &structpb.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
						},
						util.StructAnyValue: {
							Kind: &structpb.Value_StructValue{StructValue: settings.Inline.Settings},
						},
					},
				},
			},
		}
	}

	return nil
}

func (r *PluginManagerReconciler) applyConfigDiscoveryPlugin(filterName, typeURL string, defaultConfig *structpb.Struct, out *structpb.Struct) error {
	out.Fields[util.StructHttpFilterName] = &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: filterName,
		},
	}

	configDiscoveryFields := map[string]*structpb.Value{
		util.StructHttpFilterConfigSource: {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{
			util.StructHttpFilterAds: {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: map[string]*structpb.Value{}}}},
		}}}},
		util.StructHttpFilterTypeURLs: {Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{
			{Kind: &structpb.Value_StringValue{StringValue: typeURL}},
		}}}},
	}
	if defaultConfig != nil {
		configDiscoveryFields[util.StructHttpFilterDefaultConfig] = &structpb.Value{
			Kind: &structpb.Value_StructValue{StructValue: defaultConfig},
		}
	}
	out.Fields[util.StructHttpFilterConfigDiscovery] = &structpb.Value{
		Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: configDiscoveryFields}},
	}

	return nil
}

func (r *PluginManagerReconciler) addExtensionConfigPath(filterName string, value *structpb.Struct, p *v1alpha1.Plugin, target *[]translateOutputConfigPatch) error {
	out := &networkingapi.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networkingapi.EnvoyFilter_EXTENSION_CONFIG,
		Patch: &networkingapi.EnvoyFilter_Patch{
			Operation: networkingapi.EnvoyFilter_Patch_ADD,
			Value: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					util.StructHttpFilterName:        {Kind: &structpb.Value_StringValue{StringValue: filterName}},
					util.StructHttpFilterTypedConfig: {Kind: &structpb.Value_StructValue{StructValue: value}},
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

	wasmConfigStrValue, err := convertWasmConfigurationToStringValue(pluginWasm.Wasm.Settings)
	if err != nil {
		return nil, err
	}
	valueBytes, err := proto.Marshal(wasmConfigStrValue)
	if err != nil {
		return nil, err
	}

	pluginConfig.Configuration = &any.Any{
		TypeUrl: util.TypeURLStringValue,
		Value:   valueBytes,
	}

	return &envoyextensionsfilterswasmv3.Wasm{Config: pluginConfig}, nil
}

func convertWasmConfigurationToStringValue(pluginSettings *structpb.Struct) (*wrappers.StringValue, error) {
	if pluginSettings == nil { // use empty struct json string as wasm does not allow nil `configuration`
		pluginSettings = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"_string": {
					Kind: &structpb.Value_StringValue{
						StringValue: "{}",
					},
				},
			},
		}
	}

	// string类型的配置解析为 google.protobuf.StringValue
	if strField := pluginSettings.Fields["_string"]; strField != nil && len(pluginSettings.Fields) == 1 {
		if _, ok := strField.Kind.(*structpb.Value_StringValue); ok {
			// != Value_StringValue
			return &wrappers.StringValue{Value: strField.GetStringValue()}, nil
		}
	}

	// to json string to align with istio behaviour
	if settingsBytes, err := protojson.Marshal(pluginSettings); err != nil {
		return nil, err
	} else {
		return &wrappers.StringValue{Value: string(settingsBytes)}, nil
	}
}

func (r *PluginManagerReconciler) convertRiderFilterConfig(resourceName string, meta metav1.ObjectMeta, in *v1alpha1.Plugin) (*structpb.Struct, error) {
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

	datasourceStruct, err := util.MessageToStruct(datasource)
	if err != nil {
		return nil, err
	}

	riderPluginConfig := &structpb.Struct{Fields: map[string]*structpb.Value{
		"name": {Kind: &structpb.Value_StringValue{StringValue: pluginRider.Rider.PluginName}},
		"vm_config": {Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"package_path": {Kind: &structpb.Value_StringValue{StringValue: riderPackagePath}},
			},
		}}},
		"code": {Kind: &structpb.Value_StructValue{StructValue: datasourceStruct}},
	}}
	riderFilterConfig := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"plugin": {Kind: &structpb.Value_StructValue{StructValue: riderPluginConfig}},
		},
	}

	config := pluginRider.Rider.Settings
	ensureEnv := func() *structpb.Struct {
		if config.GetFields() == nil {
			config = &structpb.Struct{Fields: map[string]*structpb.Value{}}
		}

		envSt := config.Fields[RiderEnvKey].GetStructValue()
		if envSt == nil {
			envSt = &structpb.Struct{Fields: map[string]*structpb.Value{}}
			config.Fields[RiderEnvKey] = &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: envSt}}
		}
		if envSt.Fields == nil {
			envSt.Fields = map[string]*structpb.Value{}
		}
		return envSt
	}
	if imagePullSecretContent != "" {
		ensureEnv().Fields[WasmSecretEnv] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: imagePullSecretContent}}
	}

	if config != nil {
		riderPluginConfig.Fields["config"] = &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: config}}
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
						Timeout: duration.New(30 * time.Second),
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
