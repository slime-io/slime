/*
* @Author: yangdihang
* @Date: 2020/11/19
 */

package controllers

import (
	"fmt"
	"hash/adler32"
	"sort"
	"strconv"
	"strings"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	envoy_extensions_filters_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_match_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/protobuf/types/known/structpb"
	networking "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/types"

	"slime.io/slime/framework/util"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

// store the vhostName/routeName and action
// if routeName is "", it means the config is specified to vhost (contain all route in vhost)
type routeConfig struct {
	gw         bool
	action     []*envoy_config_route_v3.RateLimit_Action
	bodyAction *rlBodyAction

	routeName string
	vhostName string
	direction string
}

// vhostName/routeName,
func genVhostRouteName(rc *routeConfig) string {
	return fmt.Sprintf("%s/%s", rc.vhostName, rc.routeName)
}

func getTarget(inner, outer *microservicev1alpha2.Target) *microservicev1alpha2.Target {
	target := &microservicev1alpha2.Target{}

	if inner != nil {
		target = inner
	} else {
		target = outer
	}
	return target
}

func generateRouteConfigs(inner, outer *microservicev1alpha2.Target, gw bool) []*routeConfig {
	rcs := make([]*routeConfig, 0)

	target := getTarget(inner, outer)
	// build outbound route
	if target != nil && target.Direction == model.Outbound {
		log.Debugf("build outbound route config base on %s", target.String())
		if len(target.Route) > 0 {
			for _, route := range target.Route {
				parts := strings.Split(route, "/")
				if len(parts) != 2 {
					continue
				}
				vhostName, routeName := parts[0], parts[1]
				if target.Port != 0 {
					vhostName = fmt.Sprintf("%s:%d", vhostName, target.Port)
				}
				rc := &routeConfig{
					routeName: routeName,
					vhostName: vhostName,
					direction: target.Direction,
					gw:        gw,
				}
				rcs = append(rcs, rc)
			}
			return rcs
		} else if len(target.Host) > 0 {
			for _, vhost := range target.Host {
				vhostName := vhost
				if target.Port != 0 {
					vhostName = fmt.Sprintf("%s:%d", vhostName, target.Port)
				}
				rc := &routeConfig{
					routeName: "",
					vhostName: vhostName,
					direction: target.Direction,
					gw:        gw,
				}
				rcs = append(rcs, rc)
			}
			return rcs
		}
	}
	// build inbound by default
	log.Debugf("build default inbound route config base on %s", target)
	rcs = generateDefaultInboundRouteConfigs(target)
	return rcs
}

// if port is zero, allow any
func generateDefaultInboundRouteConfigs(target *microservicev1alpha2.Target) []*routeConfig {
	rcs := make([]*routeConfig, 0)
	if target == nil || target.Port == 0 {
		rcs = append(rcs, &routeConfig{
			routeName: model.InboundDefaultRoute,
			vhostName: model.AllowAllPort,
			direction: model.Inbound,
		})
		return rcs
	}
	rcs = append(rcs, &routeConfig{
		routeName: model.InboundDefaultRoute,
		vhostName: fmt.Sprintf("%s|%s|%d", model.Inbound, "http", target.Port),
		direction: model.Inbound,
	})
	return rcs
}

func generateHttpRouterPatch(descriptors []*microservicev1alpha2.SmartLimitDescriptor, params *LimiterSpec) ([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, error) {
	patches := make([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	// route2RouteConfig store the vhostName/routeName => routeConfig
	route2RouteConfig := make(map[string][]*routeConfig)
	routeNameList := make([]string, 0)

	for _, descriptor := range descriptors {
		rcs := generateRouteConfigs(descriptor.Target, params.target, params.gw)
		action, bodyAction := generateRouteRateLimitAction(descriptor, params.loc)
		if action == nil && bodyAction == nil {
			continue
		}

		for _, rc := range rcs {
			rc.action = action
			rc.bodyAction = bodyAction
			vHostRouteName := genVhostRouteName(rc)

			if _, ok := route2RouteConfig[vHostRouteName]; !ok {
				route2RouteConfig[vHostRouteName] = []*routeConfig{rc}
				routeNameList = append(routeNameList, vHostRouteName)
			} else {
				route2RouteConfig[vHostRouteName] = append(route2RouteConfig[vHostRouteName], rc)
			}
		}
	}
	log.Debugf("get route2RouteConfig %v", route2RouteConfig)
	for _, rn := range routeNameList {
		rcs, ok := route2RouteConfig[rn]
		if !ok {
			continue
		}

		record := make(map[int]int)
		bodyactions := make([]*rlBodyAction, 0)
		rateLimits := make([]*envoy_config_route_v3.RateLimit, 0)

		for _, rc := range rcs {
			if len(rc.action) > 0 {
				rateLimits = append(rateLimits, &envoy_config_route_v3.RateLimit{Actions: rc.action})
			}
			if rc.bodyAction != nil {
				bodyactions = append(bodyactions, rc.bodyAction)
				// if rc action is exist
				if len(rc.action) > 0 {
					record[len(rateLimits)-1] = len(bodyactions) - 1
				}
			}
		}

		if len(rateLimits) == 0 && len(bodyactions) == 0 {
			log.Infof("no rate limit action or body action in %s", rn)
			continue
		}

		match := generateEnvoyVhostMatch(rcs[0], params.proxyVersion)
		if match.Context == networking.EnvoyFilter_GATEWAY {
			params.context = model.Gateway
		} else if match.Context == networking.EnvoyFilter_SIDECAR_OUTBOUND {
			params.context = model.Outbound
		}

		patch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
			Match: match,
			Patch: &networking.EnvoyFilter_Patch{
				Operation: networking.EnvoyFilter_Patch_MERGE,
			},
		}
		patch2vhost := false
		if rcs[0].routeName == "" {
			patch2vhost = true
			// route name not specified, patch to vhost
			vh := &envoy_config_route_v3.VirtualHost{RateLimits: rateLimits}
			vhStruct, err := util.MessageToStruct(vh)
			if err != nil {
				return nil, err
			}
			patch.ApplyTo = networking.EnvoyFilter_VIRTUAL_HOST
			patch.Patch.Value = vhStruct
		} else {
			route := &envoy_config_route_v3.Route{
				Action: &envoy_config_route_v3.Route_Route{
					Route: &envoy_config_route_v3.RouteAction{
						RateLimits: rateLimits,
					},
				},
			}
			routeStruct, err := util.MessageToStruct(route)
			if err != nil {
				return nil, err
			}
			patch.ApplyTo = networking.EnvoyFilter_HTTP_ROUTE
			patch.Patch.Value = routeStruct
		}

		if len(bodyactions) > 0 {
			patchBodyActions(patch, patch2vhost, record, bodyactions)
		}

		patches = append(patches, patch)
	}
	return patches, nil
}

func patchBodyActions(patch *networking.EnvoyFilter_EnvoyConfigObjectPatch, patch2vhost bool, record map[int]int, bodyactions []*rlBodyAction) error {
	ppv := patch.Patch.Value
	m, err := util.ProtoToMap(ppv)
	if err != nil {
		return fmt.Errorf("convert rate_limits proto to map err,%+v", err.Error())
	}

	if patch2vhost {
		if rls, ok := m[model.RateLimits].([]interface{}); ok {
			rls = patchBodyActionToRate(rls, bodyactions, record)
			m[model.RateLimits] = rls
		}
	} else {
		if route, ok := m[model.Route].(map[string]interface{}); !ok {
			return fmt.Errorf("convert route to map[string]interface{} failed")
		} else if rls, ok := route[model.RateLimits].([]interface{}); !ok {
			return fmt.Errorf("convert rate_limits to []interface{} failed")
		} else {
			rls = patchBodyActionToRate(rls, bodyactions, record)
			route[model.RateLimits] = rls
		}
	}

	ts := &structpb.Struct{}
	err = util.FromJSONMapToMessage(m, ts)
	if err == nil {
		patch.Patch.Value = ts
	} else {
		return fmt.Errorf("convert map to struct err,%+v", err.Error())
	}
	return nil
}

func patchBodyActionToRate(rls []interface{}, bodyactions []*rlBodyAction, record map[int]int) []interface{} {
	log.Debugf("rls is %+v, bodyactions is %+v, record is %+v", rls, bodyactions, record)
	deleted := make([]int, 0)

	for i := range rls {
		if actions, ok := rls[i].(map[string]interface{}); ok {
			if specifier, ok := actions[model.RateLimitActions].([]interface{}); ok {
				if val, ok := record[i]; ok && val < len(bodyactions) {
					specifier = append(specifier, bodyactions[val])
					deleted = append(deleted, val)
				}
				actions[model.RateLimitActions] = specifier
			}
		}
	}

	bodyactions = deleteAtIndex(bodyactions, deleted)

	for i := range bodyactions {
		rls = append(rls, map[string]interface{}{
			model.RateLimitActions: []*rlBodyAction{bodyactions[i]},
		})
	}
	return rls
}

func deleteAtIndex(slice []*rlBodyAction, indices []int) []*rlBodyAction {
	sort.Sort(sort.Reverse(sort.IntSlice(indices)))
	for _, idx := range indices {
		if idx >= 0 && idx < len(slice) {
			slice = append(slice[:idx], slice[idx+1:]...)
		}
	}
	return slice
}

// only enable local rate limit
func generateHttpFilterLocalRateLimitPatch(context, proxyVersion string) *networking.EnvoyFilter_EnvoyConfigObjectPatch {
	localRateLimit := &envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix: util.StructEnvoyLocalRateLimitLimiter,
	}
	local, err := util.MessageToStruct(localRateLimit)
	if err != nil {
		log.Errorf("can not be here, convert message to struct err,%+v", err.Error())
		return nil
	}

	patch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
		Match:   generateEnvoyHttpFilterMatch(context, proxyVersion),
		Patch: &networking.EnvoyFilter_Patch{
			Operation: networking.EnvoyFilter_Patch_INSERT_BEFORE,
			Value: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					util.StructHttpFilterName: {
						Kind: &structpb.Value_StringValue{StringValue: util.EnvoyLocalRateLimit},
					},
					util.StructHttpFilterTypedConfig: {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									util.StructAnyAtType: {
										Kind: &structpb.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
									},
									util.StructAnyTypeURL: {
										Kind: &structpb.Value_StringValue{StringValue: util.TypeURLEnvoyLocalRateLimit},
									},
									util.StructAnyValue: {
										Kind: &structpb.Value_StructValue{StructValue: local},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return patch
}

func generateLocalRateLimitPerFilterPatch(descriptors []*microservicev1alpha2.SmartLimitDescriptor, params *LimiterSpec) []*networking.EnvoyFilter_EnvoyConfigObjectPatch {
	patches := make([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	route2Descriptors := make(map[string][]*microservicev1alpha2.SmartLimitDescriptor)
	route2RouteConfig := make(map[string][]*routeConfig)
	routeNameList := make([]string, 0)

	for _, descriptor := range descriptors {
		rcs := generateRouteConfigs(descriptor.Target, params.target, params.gw)
		for _, rc := range rcs {
			vHostRouteName := genVhostRouteName(rc)

			if _, ok := route2Descriptors[vHostRouteName]; !ok {
				route2Descriptors[vHostRouteName] = []*microservicev1alpha2.SmartLimitDescriptor{descriptor}
				routeNameList = append(routeNameList, vHostRouteName)
			} else {
				route2Descriptors[vHostRouteName] = append(route2Descriptors[vHostRouteName], descriptor)
			}

			if _, ok := route2RouteConfig[vHostRouteName]; !ok {
				route2RouteConfig[vHostRouteName] = []*routeConfig{rc}
			} else {
				route2RouteConfig[vHostRouteName] = append(route2RouteConfig[vHostRouteName], rc)
			}
		}
	}

	for _, vr := range routeNameList {
		desc, ok := route2Descriptors[vr]
		if !ok {
			continue
		}
		rcs, ok := route2RouteConfig[vr]
		if !ok {
			continue
		}

		// build token bucket
		localRateLimitDescriptors, existBodyMatch := generateLocalRateLimitDescriptors(desc, params.loc)
		localRateLimit := &envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit{
			TokenBucket:    generateCustomTokenBucket(100000, 100000, 1),
			Descriptors:    localRateLimitDescriptors,
			StatPrefix:     util.StructEnvoyLocalRateLimitLimiter,
			FilterEnabled:  generateEnvoyLocalRateLimitEnabled(),
			FilterEnforced: generateEnvoyLocalRateLimitEnforced(),
		}
		headers := generateResponseHeaderToAdd(desc)
		if len(headers) > 0 {
			localRateLimit.ResponseHeadersToAdd = headers
		}

		local, err := util.MessageToStruct(localRateLimit)
		if err != nil {
			return nil
		}

		if existBodyMatch {
			local, err = patchBodyMatchToRateLimitDescriptors(local)
			if err != nil {
				log.Errorf("patch body match to rate limit descriptors err,%+v", err.Error())
				return nil
			}
		}

		patch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
			Match: generateEnvoyVhostMatch(rcs[0], params.proxyVersion),
			Patch: generatePerFilterPatch(local),
		}
		if rcs[0].routeName == "" {
			patch.ApplyTo = networking.EnvoyFilter_VIRTUAL_HOST
		} else {
			patch.ApplyTo = networking.EnvoyFilter_HTTP_ROUTE
		}
		patches = append(patches, patch)
	}
	return patches
}

func patchBodyMatchToRateLimitDescriptors(ss *structpb.Struct) (*structpb.Struct, error) {
	m, err := util.ProtoToMap(ss)
	if err != nil {
		return nil, fmt.Errorf("convert ratelimiter descriptors proto to map err,%+v", err.Error())
	}
	// set body_match to true
	m[model.BodyMatch] = true
	res := &structpb.Struct{}
	err = util.FromJSONMapToMessage(m, res)
	if err != nil {
		return nil, fmt.Errorf("convert ratelimiter descriptors map to proto err,%+v", err.Error())
	}
	return res, nil
}

/*
// if key/value is not empty, envoyplugin is needed, we will not generate http route patch
// 有match时，只有当header中的值与match相匹配才会进行对路由进行action限流，需要注意的是RegexMatch(name 的值是否匹配正则)与
// PresentMatch(name是否存在)互斥
// 这里之前打算在pb声明为oneof,但是用kubebuilder生成api的过程中无法识别相关interface{}
*/
func generateRouteRateLimitAction(descriptor *microservicev1alpha2.SmartLimitDescriptor, loc types.NamespacedName) ([]*envoy_config_route_v3.RateLimit_Action, *rlBodyAction) {
	actions := make([]*envoy_config_route_v3.RateLimit_Action, 0)
	var bodyAction *rlBodyAction

	asGlobalLimiter := false
	if descriptor.Action.Strategy == model.GlobalSmartLimiter {
		asGlobalLimiter = true
	}

	if descriptor.CustomKey != "" && descriptor.CustomValue != "" {
		log.Infof("customKey/customValue is not empty, users should apply a envoyplugin with same customKey/customValue")
		return nil, bodyAction
	} else if len(descriptor.Match) == 0 {
		// no match specified in smartLimiter, gen DescriptorValue as normal
		action := &envoy_config_route_v3.RateLimit_Action{}
		action.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_GenericKey_{
			GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
				DescriptorValue: generateDescriptorValue(descriptor, loc),
			},
		}
		return []*envoy_config_route_v3.RateLimit_Action{action}, bodyAction
	} else {
		// match is specified in smartLimiter
		headers := make([]*envoy_config_route_v3.HeaderMatcher, 0)
		queries := make([]*envoy_config_route_v3.QueryParameterMatcher, 0)
		bodies := make([]*rlBodyMatch, 0)

		ips := make([]string, 0)

		for _, match := range descriptor.Match {
			useHeader, useQuery, useJsonBody, useSourceIp := false, false, false, false

			switch match.MatchSource {
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_SourceIpMatch:
				useSourceIp = true
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_JsonBodyMatch:
				useJsonBody = true
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_QueryMatch:
				useQuery = true
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_HeadMatch:
				// compatible with old version
				if match.UseQueryMatch {
					useQuery = true
				} else {
					useHeader = true
				}
			}

			if useSourceIp {
				ips = append(ips, match.ExactMatch)
			} else if useJsonBody {
				if bodymatch, err := generateBodyMatch(match); err == nil {
					bodies = append(bodies, &bodymatch)
				}
			} else if useQuery {
				if query, err := generateQueryMatchAction(match); err == nil {
					queries = append(queries, query)
				}
			} else if match.PresentMatchSeparate {
				// Special cases to generate requestHeader and headerMatch,
				log.Debugf("PresentMatchSeparate is specifed in smartLimiter")
				return generatePresentMatchSeparate(match, descriptor, loc), bodyAction
			} else if useHeader {
				if header, err := generateHeaderMatchAction(match); err == nil {
					headers = append(headers, header)
				}
			}
		}

		// sequence of actions is important
		// it should same with the sequence of bucket entries

		// suquence:  query > header > sourceIp

		if len(queries) > 0 {
			queryAction := &envoy_config_route_v3.RateLimit_Action{}
			queryAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_QueryParameterValueMatch_{
				QueryParameterValueMatch: &envoy_config_route_v3.RateLimit_Action_QueryParameterValueMatch{
					DescriptorValue: generateDescriptorValue(descriptor, loc),
					QueryParameters: queries,
				},
			}
			actions = append(actions, queryAction)
		}

		if len(headers) > 0 {
			headerAction := &envoy_config_route_v3.RateLimit_Action{}
			headerAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch_{
				HeaderValueMatch: &envoy_config_route_v3.RateLimit_Action_HeaderValueMatch{
					DescriptorValue: generateDescriptorValue(descriptor, loc),
					Headers:         headers,
				},
			}
			actions = append(actions, headerAction)
		}

		if len(ips) > 0 {
			// add genericKey action to identify the request in global limiter
			if asGlobalLimiter {
				action := &envoy_config_route_v3.RateLimit_Action{}
				action.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_GenericKey_{
					GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
						DescriptorValue: generateDescriptorValue(descriptor, loc),
					},
				}
				actions = append(actions, action)
			}

			sourceIpAction := &envoy_config_route_v3.RateLimit_Action{}
			sourceIpAction.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RemoteAddress_{
				RemoteAddress: &envoy_config_route_v3.RateLimit_Action_RemoteAddress{},
			}
			actions = append(actions, sourceIpAction)
		}

		if len(bodies) > 0 {
			bodyAction = &rlBodyAction{}
			bodyAction.Body_value_match = map[string]interface{}{
				model.DescriptiorValue: generateDescriptorValue(descriptor, loc),
				model.Bodies:           bodies,
			}
		}
	}
	return actions, bodyAction
}

func generateLocalRateLimitDescriptors(descriptors []*microservicev1alpha2.SmartLimitDescriptor, loc types.NamespacedName) ([]*envoy_ratelimit_v3.LocalRateLimitDescriptor, bool) {
	localRateLimitDescriptors := make([]*envoy_ratelimit_v3.LocalRateLimitDescriptor, 0)
	existBodyMatch := false
	for _, item := range descriptors {
		entries, exist := generateLocalRateLimitDescriptorEntries(item, loc)
		if exist {
			existBodyMatch = true
		}
		tokenBucket := generateTokenBucket(item)
		localRateLimitDescriptors = append(localRateLimitDescriptors, &envoy_ratelimit_v3.LocalRateLimitDescriptor{
			Entries:     entries,
			TokenBucket: tokenBucket,
		})
	}
	return localRateLimitDescriptors, existBodyMatch
}

// generateLocalRateLimitDescriptorEntries gen entries like above
func generateLocalRateLimitDescriptorEntries(des *microservicev1alpha2.SmartLimitDescriptor, loc types.NamespacedName) ([]*envoy_ratelimit_v3.RateLimitDescriptor_Entry, bool) {
	var useJsonBody, useQuery, useHeader, useSourceIp bool
	entry := &envoy_ratelimit_v3.RateLimitDescriptor_Entry{}
	entries := make([]*envoy_ratelimit_v3.RateLimitDescriptor_Entry, 0)

	if des.CustomKey != "" && des.CustomValue != "" {
		entry.Key = des.CustomKey
		entry.Value = des.CustomValue
		entries = append(entries, entry)
	} else if len(des.Match) == 0 {
		entry.Key = model.GenericKey
		entry.Value = generateDescriptorValue(des, loc)
		entries = append(entries, entry)
	} else {
		for _, match := range des.Match {
			switch match.MatchSource {
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_SourceIpMatch:
				useSourceIp = true
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_JsonBodyMatch:
				useJsonBody = true
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_QueryMatch:
				useQuery = true
			case microservicev1alpha2.SmartLimitDescriptor_Matcher_HeadMatch:
				// compatible with old version
				if match.UseQueryMatch {
					useQuery = true
				} else {
					useHeader = true
				}
			}
		}

		// suquence:  query > header > sourceIp

		if useQuery {
			item := &envoy_ratelimit_v3.RateLimitDescriptor_Entry{}
			item.Key = model.QueryMatch
			item.Value = generateDescriptorValue(des, loc)
			entries = append(entries, item)
		}

		if useHeader {
			item := &envoy_ratelimit_v3.RateLimitDescriptor_Entry{}
			item.Key = model.HeaderValueMatch
			item.Value = generateDescriptorValue(des, loc)
			entries = append(entries, item)
		}

		if useSourceIp {
			item := &envoy_ratelimit_v3.RateLimitDescriptor_Entry{}
			item.Key = model.RemoteAddress
			item.Value = generateRemoteAddressDescriptorValue(des)
			entries = append(entries, item)
		}

		if useJsonBody {
			item := &envoy_ratelimit_v3.RateLimitDescriptor_Entry{}
			item.Key = model.BodyMatch
			item.Value = generateDescriptorValue(des, loc)
			entries = append(entries, item)
		}
	}
	return entries, useJsonBody
}

func generateTokenBucket(item *microservicev1alpha2.SmartLimitDescriptor) *envoy_type_v3.TokenBucket {
	i, _ := strconv.Atoi(item.Action.Quota)
	return &envoy_type_v3.TokenBucket{
		MaxTokens: uint32(i),
		FillInterval: &duration.Duration{
			Seconds: int64(item.Action.FillInterval.Seconds),
			Nanos:   item.Action.FillInterval.Nanos,
		},
		TokensPerFill: &wrappers.UInt32Value{Value: uint32(i)},
	}
}

func generateDescriptorValue(item *microservicev1alpha2.SmartLimitDescriptor, loc types.NamespacedName) string {
	id := adler32.Checksum([]byte(item.String() + loc.String()))
	return fmt.Sprintf("Service[%s.%s]-Id[%d]", loc.Name, loc.Namespace, id)
}

// only support one sourceIp in descriptor, if multiple sourceIp is set, the first one is used
func generateRemoteAddressDescriptorValue(item *microservicev1alpha2.SmartLimitDescriptor) string {
	for _, match := range item.Match {
		if match.MatchSource == microservicev1alpha2.SmartLimitDescriptor_Matcher_SourceIpMatch {
			return match.ExactMatch
		}
	}
	// can not reach here
	return model.MockSourceIp
}

func generateSafeRegexMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.HeaderMatcher_SafeRegexMatch {
	return &envoy_config_route_v3.HeaderMatcher_SafeRegexMatch{
		SafeRegexMatch: &envoy_match_v3.RegexMatcher{
			EngineType: &envoy_match_v3.RegexMatcher_GoogleRe2{
				GoogleRe2: &envoy_match_v3.RegexMatcher_GoogleRE2{},
			},
			Regex: match.RegexMatch,
		},
	}
}

func generatePrefixMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.HeaderMatcher_PrefixMatch {
	return &envoy_config_route_v3.HeaderMatcher_PrefixMatch{PrefixMatch: match.PrefixMatch}
}

func generateSuffixMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.HeaderMatcher_SuffixMatch {
	return &envoy_config_route_v3.HeaderMatcher_SuffixMatch{SuffixMatch: match.SuffixMatch}
}

func generateExactMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.HeaderMatcher_ExactMatch {
	return &envoy_config_route_v3.HeaderMatcher_ExactMatch{ExactMatch: match.ExactMatch}
}

func generateInvertMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) bool {
	return match.InvertMatch
}

func generatePresentMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.HeaderMatcher_PresentMatch {
	return &envoy_config_route_v3.HeaderMatcher_PresentMatch{PresentMatch: match.PresentMatch}
}

// TODO
func generateCustomTokenBucket(maxTokens, tokensPerFill, second int) *envoy_type_v3.TokenBucket {
	return &envoy_type_v3.TokenBucket{
		MaxTokens: uint32(maxTokens),
		FillInterval: &duration.Duration{
			Seconds: int64(second),
		},
		TokensPerFill: &wrappers.UInt32Value{Value: uint32(tokensPerFill)},
	}
}

// % of requests that will check the local rate limit decision, but not enforce,
// for a given route_key specified in the local rate limit configuration. Defaults to 0.
func generateEnvoyLocalRateLimitEnabled() *envoy_core_v3.RuntimeFractionalPercent {
	return &envoy_core_v3.RuntimeFractionalPercent{
		RuntimeKey: util.StructEnvoyLocalRateLimitEnabled,
		DefaultValue: &envoy_type_v3.FractionalPercent{
			Numerator:   100,
			Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
		},
	}
}

//	% of requests that will enforce the local rate limit decision for a given route_key specified in the local rate limit configuration.
//
// Defaults to 0. This can be used to test what would happen before fully enforcing the outcome.
func generateEnvoyLocalRateLimitEnforced() *envoy_core_v3.RuntimeFractionalPercent {
	return &envoy_core_v3.RuntimeFractionalPercent{
		RuntimeKey: util.StructEnvoyLocalRateLimitEnforced,
		DefaultValue: &envoy_type_v3.FractionalPercent{
			Numerator:   100,
			Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
		},
	}
}

func generateResponseHeaderToAdd(items []*microservicev1alpha2.SmartLimitDescriptor) []*envoy_core_v3.HeaderValueOption {
	headers := make([]*envoy_core_v3.HeaderValueOption, 0)
	for _, item := range items {
		for _, item := range item.Action.HeadersToAdd {
			headers = append(headers, &envoy_core_v3.HeaderValueOption{
				Header: &envoy_core_v3.HeaderValue{
					Key:   item.GetKey(),
					Value: item.GetValue(),
				},
				Append: &wrappers.BoolValue{
					Value: true,
				},
			})
		}
	}
	return headers
}

func generateEnvoyVhostMatch(rc *routeConfig, proxyVersion string) *networking.EnvoyFilter_EnvoyConfigObjectMatch {
	// default context is inbound
	match := &networking.EnvoyFilter_EnvoyConfigObjectMatch{
		Context: networking.EnvoyFilter_SIDECAR_INBOUND,
		ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
			RouteConfiguration: &networking.EnvoyFilter_RouteConfigurationMatch{
				Vhost: &networking.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{
					Route: &networking.EnvoyFilter_RouteConfigurationMatch_RouteMatch{
						Name: rc.routeName,
					},
				},
			},
		},
	}

	if proxyVersion != "" {
		match.Proxy = &networking.EnvoyFilter_ProxyMatch{
			ProxyVersion: proxyVersion,
		}
	}

	// if gateway is enabled, match context should be EnvoyFilter_GATEWAY
	if rc.gw {
		match.Context = networking.EnvoyFilter_GATEWAY
		log.Debugf("gw is true, set context to gateway")
	} else if rc.direction == model.Outbound {
		match.Context = networking.EnvoyFilter_SIDECAR_OUTBOUND
		log.Debugf("direction is outbound and gw is false, set context to outbound")
	}
	// if allow_any, config.RouteConfiguration.Vhost.Name is ""
	config, ok := match.ObjectTypes.(*networking.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration)
	if !ok {
		log.Errorf("covert to EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration err, can not be here")
		return match
	}

	if rc.routeName == "" {
		config.RouteConfiguration.Vhost.Route = nil
	}
	config.RouteConfiguration.Vhost.Name = rc.vhostName

	return match
}

func generatePerFilterPatch(local *structpb.Struct) *networking.EnvoyFilter_Patch {
	return &networking.EnvoyFilter_Patch{
		Operation: networking.EnvoyFilter_Patch_MERGE,
		Value: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				model.TypePerFilterConfig: {
					Kind: &structpb.Value_StructValue{
						StructValue: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								util.EnvoyLocalRateLimit: {
									Kind: &structpb.Value_StructValue{
										StructValue: &structpb.Struct{
											Fields: map[string]*structpb.Value{
												util.StructAnyAtType: {
													Kind: &structpb.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
												},
												util.StructAnyTypeURL: {
													Kind: &structpb.Value_StringValue{StringValue: util.TypeURLEnvoyLocalRateLimit},
												},
												util.StructAnyValue: {
													Kind: &structpb.Value_StructValue{StructValue: local},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

type rlBodyAction struct {
	Body_value_match map[string]interface{} `json:"body_value_match,omitempty"`
}

type rlBodyMatch struct {
	Name         string                 `json:"name,omitempty"`
	String_match map[string]interface{} `json:"string_match,omitempty"`
}

// support exact|prefix|suffix
func generateBodyMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) (rlBodyMatch, error) {
	var err error
	val := rlBodyMatch{Name: match.Name}

	switch {
	case match.ExactMatch != "":
		val.String_match = map[string]interface{}{
			"exact": match.ExactMatch,
		}
	case match.PrefixMatch != "":
		val.String_match = map[string]interface{}{
			"prefix": match.PrefixMatch,
		}
	case match.SuffixMatch != "":
		val.String_match = map[string]interface{}{
			"suffix": match.SuffixMatch,
		}
	case match.RegexMatch != "":
		val.String_match = map[string]interface{}{
			"safe_regex": map[string]interface{}{
				"google_re2": map[string]interface{}{},
				"regex":      match.RegexMatch,
			},
		}
	default:
		err = fmt.Errorf("unsupport %s in body match", match.Name)
	}
	return val, err
}

// generateQueryMatchAction	gen query match in rateLimit action
func generateQueryMatchAction(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) (*envoy_config_route_v3.QueryParameterMatcher, error) {
	var err error
	query := &envoy_config_route_v3.QueryParameterMatcher{}

	query.Name = match.Name
	switch {
	case match.RegexMatch != "":
		query.QueryParameterMatchSpecifier = generateQuerySafeRegexMatch(match)
	case match.ExactMatch != "":
		query.QueryParameterMatchSpecifier = generateQueryExactMatch(match)
	case match.PrefixMatch != "":
		query.QueryParameterMatchSpecifier = generateQueryPrefixMatch(match)
	case match.SuffixMatch != "":
		query.QueryParameterMatchSpecifier = generateQuerySuffixMatch(match)
	default:
		err = fmt.Errorf("unknown query match type %s", query.Name)
	}
	return query, err
}

// generateHeaderMatchAction gen header match in rateLimit action
func generateHeaderMatchAction(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) (*envoy_config_route_v3.HeaderMatcher, error) {
	var err error
	header := &envoy_config_route_v3.HeaderMatcher{}

	header.Name = match.Name
	header.InvertMatch = generateInvertMatch(match)
	switch {
	case match.RegexMatch != "":
		header.HeaderMatchSpecifier = generateSafeRegexMatch(match)
	case match.ExactMatch != "":
		header.HeaderMatchSpecifier = generateExactMatch(match)
	case match.PrefixMatch != "":
		header.HeaderMatchSpecifier = generatePrefixMatch(match)
	case match.SuffixMatch != "":
		header.HeaderMatchSpecifier = generateSuffixMatch(match)
	case match.IsExactMatchEmpty:
		header.HeaderMatchSpecifier = generateExactMatch(match)
	case match.PresentMatch:
		header.HeaderMatchSpecifier = generatePresentMatch(match)
	default:
		err = fmt.Errorf("unknown query match type %s", header.Name)
	}
	return header, err
}

func generateQuerySafeRegexMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.QueryParameterMatcher_StringMatch {
	return &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
		StringMatch: &envoy_match_v3.StringMatcher{
			MatchPattern: &envoy_match_v3.StringMatcher_SafeRegex{SafeRegex: &envoy_match_v3.RegexMatcher{
				EngineType: &envoy_match_v3.RegexMatcher_GoogleRe2{
					GoogleRe2: &envoy_match_v3.RegexMatcher_GoogleRE2{},
				},
				Regex: match.RegexMatch,
			}},
		},
	}
}

func generateQueryPrefixMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.QueryParameterMatcher_StringMatch {
	return &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
		StringMatch: &envoy_match_v3.StringMatcher{
			MatchPattern: &envoy_match_v3.StringMatcher_Prefix{Prefix: match.PrefixMatch},
		},
	}
}

func generateQuerySuffixMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.QueryParameterMatcher_StringMatch {
	return &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
		StringMatch: &envoy_match_v3.StringMatcher{
			MatchPattern: &envoy_match_v3.StringMatcher_Suffix{Suffix: match.SuffixMatch},
		},
	}
}

func generateQueryExactMatch(match *microservicev1alpha2.SmartLimitDescriptor_Matcher) *envoy_config_route_v3.QueryParameterMatcher_StringMatch {
	return &envoy_config_route_v3.QueryParameterMatcher_StringMatch{
		StringMatch: &envoy_match_v3.StringMatcher{
			MatchPattern: &envoy_match_v3.StringMatcher_Exact{Exact: match.ExactMatch},
		},
	}
}

// global support PresentMatchSeparate
func generatePresentMatchSeparate(match *microservicev1alpha2.SmartLimitDescriptor_Matcher,
	descriptor *microservicev1alpha2.SmartLimitDescriptor,
	loc types.NamespacedName,
) []*envoy_config_route_v3.RateLimit_Action {
	action := &envoy_config_route_v3.RateLimit_Action{}
	action.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_RequestHeaders_{
		RequestHeaders: &envoy_config_route_v3.RateLimit_Action_RequestHeaders{
			HeaderName:    match.Name,
			DescriptorKey: generateDescriptorKey(descriptor, loc),
		},
	}
	generic := &envoy_config_route_v3.RateLimit_Action{}
	generic.ActionSpecifier = &envoy_config_route_v3.RateLimit_Action_GenericKey_{
		GenericKey: &envoy_config_route_v3.RateLimit_Action_GenericKey{
			DescriptorValue: generateDescriptorValue(descriptor, loc),
		},
	}
	return []*envoy_config_route_v3.RateLimit_Action{generic, action}
}
