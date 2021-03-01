/*
* @Author: yangdihang
* @Date: 2020/11/5
 */
package smartlimiter

// TODO: Since the com.netease.local_flow_control has not yet been opened, this function is disabled

/*
import (
	"context"
	"fmt"
	"hash/fnv"

	envoy_api_v2_route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	structpb "github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/wrappers"
	networking "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"slime.io/slime/pkg/apis/microservice/v1alpha1"
	microservicev1alpha1 "slime.io/slime/pkg/apis/microservice/v1alpha1"
	"slime.io/slime/pkg/apis/networking/v1alpha3"
	"slime.io/slime/pkg/util"
)

func HeaderMatchToKey(matchers []*v1alpha1.HeaderMatcher) string {
	h := fnv.New64()
	var ms string
	for _, matcher := range matchers {
		ms = ms + fmt.Sprintf("%v", matcher)
	}
	_, _ = h.Write([]byte(ms))
	return fmt.Sprintf("%d", h.Sum64())
}

func TransformHeaderMatcher(matchers []*v1alpha1.HeaderMatcher) []*envoy_api_v2_route.HeaderMatcher {
	ms := make([]*envoy_api_v2_route.HeaderMatcher, 0)
	for _, matcher := range matchers {
		x := &envoy_api_v2_route.HeaderMatcher{
			InvertMatch: matcher.InvertMatch,
		}
		if matcher.ExactMatch != "" {
			x.HeaderMatchSpecifier = &envoy_api_v2_route.HeaderMatcher_ExactMatch{
				ExactMatch: matcher.ExactMatch,
			}
		}
		if matcher.PrefixMatch != "" {
			x.HeaderMatchSpecifier = &envoy_api_v2_route.HeaderMatcher_PrefixMatch{
				PrefixMatch: matcher.PrefixMatch,
			}
		}
		if matcher.SuffixMatch != "" {
			x.HeaderMatchSpecifier = &envoy_api_v2_route.HeaderMatcher_SuffixMatch{
				SuffixMatch: matcher.SuffixMatch,
			}
		}
		if matcher.RegexMatch != "" {
			x.HeaderMatchSpecifier = &envoy_api_v2_route.HeaderMatcher_RegexMatch{
				RegexMatch: matcher.ExactMatch,
			}
		}
		x.InvertMatch = matcher.InvertMatch
		x.Name = matcher.Name
		ms = append(ms, x)
	}
	return ms
}

func figureOutStatus(descriptor *microservicev1alpha1.SmartLimitDescriptor, material map[string]string, matchKey string) *microservicev1alpha1.RateLimitDescriptorConfigStatus {
	if shouldUpdate, _ := util.CalculateTemplateBool(descriptor.Condition, material); shouldUpdate {
		if descriptor.Action != nil {
			if rateLimitValue, err := util.CalculateTemplate(descriptor.Action.Quota, material); err == nil {
				quota := uint32(rateLimitValue) / uint32(descriptor.Action.FillInterval.Seconds)
				newStatus := &microservicev1alpha1.RateLimitDescriptorConfigStatus{
					Key:   MATCH_KEY,
					Value: matchKey,
					RateLimit: &microservicev1alpha1.RateConfig{
						RequestsPerUnit: quota,
						Unit:            microservicev1alpha1.UnitType_SECOND,
					},
				}
				return newStatus
			}
		}

	}
	return nil
}

func figureOutAction(matchers []*microservicev1alpha1.HeaderMatcher, key string) *envoy_api_v2_route.RateLimit_Action {
	a := &envoy_api_v2_route.RateLimit_Action{
		ActionSpecifier: &envoy_api_v2_route.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &envoy_api_v2_route.RateLimit_Action_HeaderValueMatch{
				DescriptorValue: key,
				Headers:         TransformHeaderMatcher(matchers),
			},
		},
	}
	return a
}

func GenerateNeteaseFlowControlEnvoyFilter(cr *v1alpha1.SmartLimiter, r *ReconcileSmartLimiter, actions []*envoy_api_v2_route.RateLimit_Action, status *v1alpha1.FlowControlConfStatus) *v1alpha3.EnvoyFilter {
	loc := types.NamespacedName{
		Namespace: cr.Namespace,
		Name:      cr.Name,
	}
	svc := &v1.Service{}
	_ = r.client.Get(context.TODO(), loc, svc)
	ef := &networking.EnvoyFilter{
		WorkloadSelector: &networking.WorkloadSelector{
			Labels: svc.Spec.Selector,
		},
	}

	// 异常处理
	name := svcToName(svc)
	if name == "" {
		// TODO LOG
		return nil
	}

	if actions == nil {
		// TODO LOG
		return nil

	}

	if status == nil {
		// TODO LOG
		return nil

	}

	// 生成匹配段的EnvoyFilter
	rt := &envoy_api_v2_route.RateLimit{
		Stage: &wrappers.UInt32Value{
			// TODO: 支持更多阶段
			Value: 0,
		},
		Actions: actions,
	}
	rtStruct, _ := util.MessageToStruct(rt)
	EnvoyRatelimitPatch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networking.EnvoyFilter_VIRTUAL_HOST,
		Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
			ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
				RouteConfiguration: &networking.EnvoyFilter_RouteConfigurationMatch{
					Vhost: &networking.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{
						Name: name,
					},
				},
			},
		},
		Patch: &networking.EnvoyFilter_Patch{
			Operation: networking.EnvoyFilter_Patch_MERGE,
			Value:     rtStruct,
		},
	}

	// 生成操作段的EnvoyFilter
	s, _ := util.MessageToStruct(status)
	EnvoyFlowControlPatch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
		Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
			Context: networking.EnvoyFilter_SIDECAR_INBOUND,
			ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
				Listener: &networking.EnvoyFilter_ListenerMatch{
					FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
						Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
							Name: util.Envoy_HttpConnectionManager,
							SubFilter: &networking.EnvoyFilter_ListenerMatch_SubFilterMatch{
								Name: util.Envoy_Route,
							},
						},
					},
				},
			},
		},
		Patch: &networking.EnvoyFilter_Patch{
			Operation: networking.EnvoyFilter_Patch_INSERT_BEFORE,
			Value: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					util.Struct_HttpFilter_Name: {
						Kind: &structpb.Value_StringValue{StringValue: util.Netease_LocalFlowControl},
					},
					util.Struct_HttpFilter_TypedConfig: {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									util.Struct_Any_AtType: {
										Kind: &structpb.Value_StringValue{StringValue: util.TypeUrl_UdpaTypedStruct},
									},
									util.Struct_Any_TypedUrl: {
										Kind: &structpb.Value_StringValue{StringValue: util.TypeUrl_NeteaseLocalFlowControl},
									},
									util.Struct_Any_Value: {
										Kind: &structpb.Value_StructValue{StructValue: s},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	ef.ConfigPatches = append(ef.ConfigPatches, EnvoyRatelimitPatch, EnvoyFlowControlPatch)
	if mi, err := util.ProtoToMap(ef); err == nil {
		return &v1alpha3.EnvoyFilter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Name + "." + cr.Namespace + "." + "local-flow-control",
				Namespace: cr.Namespace,
			},
			Spec: mi,
		}
	} else {
		// TODO log
		return nil
	}
}

func (r *ReconcileSmartLimiter) GenerateNeteaseFlowControl(rateLimitConf microservicev1alpha1.SmartLimiterSpec,
	material map[string]string, instance *microservicev1alpha1.SmartLimiter) (
	*v1alpha3.EnvoyFilter, []*microservicev1alpha1.SmartLimitDescriptor) {
	l := len(rateLimitConf.Descriptors)
	actions := make([]*envoy_api_v2_route.RateLimit_Action, 0, l)
	rootDes := make([]*microservicev1alpha1.RateLimitDescriptorConfigStatus, 0, l)
	descriptor := make([]*microservicev1alpha1.SmartLimitDescriptor, 0, l)

	for _, des := range rateLimitConf.Descriptors {
		podDes := &microservicev1alpha1.SmartLimitDescriptor{}
		key := HeaderMatchToKey(des.Match)
		action := figureOutAction(des.Match, key)
		if action != nil {
			podDes.Match = des.Match
			actions = append(actions, action)
		}
		status := figureOutStatus(des, material, key)
		if status != nil {
			podDes.Action = &microservicev1alpha1.Action{
				Quota: fmt.Sprintf("%d", status.RateLimit.RequestsPerUnit),
				FillInterval: &structpb.Duration{
					Seconds: 1,
				},
			}
			rootDes = append(rootDes, status)
		}
		descriptor = append(descriptor, podDes)
	}
	// todo
	if len(rootDes) == 0 {
		return nil, nil
	}
	flowControlConf := &microservicev1alpha1.FlowControlConfStatus{
		RateLimitConf: &microservicev1alpha1.RateLimitConfStatus{
			Descriptors: rootDes,
		},
	}
	ef := GenerateNeteaseFlowControlEnvoyFilter(instance, r, actions, flowControlConf)
	return ef, descriptor
}
*/
