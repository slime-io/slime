/*
* @Author: yangdihang
* @Date: 2020/11/19
 */

package smartlimiter

import (
	"context"
	"fmt"
	"strconv"

	microservicev1alpha1 "yun.netease.com/slime/pkg/apis/microservice/v1alpha1"
	"yun.netease.com/slime/pkg/controller/destinationrule"
	"yun.netease.com/slime/pkg/util"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_filters_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	structpb "github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/duration"
	networking "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileSmartLimiter) GenerateEnvoyLocalLimit(rateLimitConf microservicev1alpha1.SmartLimiterSpec,
	material map[string]string, instance *microservicev1alpha1.SmartLimiter) (
	map[string]*networking.EnvoyFilter, map[string]*microservicev1alpha1.SmartLimitDescriptors) {

	materialInterface := util.MapToMapInterface(material)

	setsEnvoyFilter := make(map[string]*networking.EnvoyFilter)
	setsSmartLimitDescriptor := make(map[string]*microservicev1alpha1.SmartLimitDescriptors)

	host := util.UnityHost(instance.Name, instance.Namespace)
	if destinationrule.HostSubsetMapping.Get(host) != nil {
		sets := destinationrule.HostSubsetMapping.Get(host).([]*networking.Subset)
		loc := types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      instance.Name,
		}
		svc := &v1.Service{}
		_ = r.client.Get(context.TODO(), loc, svc)
		svcSelector := svc.Spec.Selector
		// 使用base作为key，可以为基础集合配置限流
		sets = append(sets, &networking.Subset{Name: util.Wellkonw_BaseSet})
   		for _, set := range sets {
			if setDescriptor, ok := rateLimitConf.Sets[set.Name]; ok {
				descriptor := &microservicev1alpha1.SmartLimitDescriptors{}
				for _, des := range setDescriptor.Descriptor_ {
					setDes := &microservicev1alpha1.SmartLimitDescriptor{}
					if shouldUpdate, _ := util.CalculateTemplateBool(des.Condition, materialInterface); shouldUpdate {
						if des.Action != nil {
							if rateLimitValue, err := util.CalculateTemplate(des.Action.Quota, materialInterface); err == nil {
								setDes.Action = &microservicev1alpha1.SmartLimitDescriptor_Action{
									Quota:        fmt.Sprintf("%d", rateLimitValue),
									FillInterval: des.Action.FillInterval,
								}
							}
							descriptor.Descriptor_ = append(descriptor.Descriptor_, setDes)
						}
					}
				}
				selector := util.CopyMap(svcSelector)
				for k, v := range set.Labels {
					selector[k] = v
				}
				if len(descriptor.Descriptor_) > 0 {
					ef := descriptorsToEnvoyFilter(descriptor.Descriptor_, selector)
					setsEnvoyFilter[set.Name] = ef
					setsSmartLimitDescriptor[set.Name] = descriptor
				}else{
					// Used to delete
					setsEnvoyFilter[set.Name] = nil
				}
			} else {
				// Used to delete
				setsEnvoyFilter[set.Name] = nil
			}
		}
		return setsEnvoyFilter, setsSmartLimitDescriptor
	}
	return nil, nil
}

func descriptorsToEnvoyFilter(descriptor []*microservicev1alpha1.SmartLimitDescriptor, labels map[string]string) *networking.EnvoyFilter {
	ef := &networking.EnvoyFilter{
		WorkloadSelector: &networking.WorkloadSelector{
			Labels: labels,
		},
	}
	ef.ConfigPatches = make([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	// envoy local ratelimit 不支持header match，因此仅应存在一条
	des := descriptor[0]
	i, _ := strconv.Atoi(des.Action.Quota)
	envoyLocDes := &envoy_extensions_filters_http_local_ratelimit_v3.LocalRateLimit{
		StatPrefix: util.Struct_EnvoyLocalRateLimit_Limiter,
		TokenBucket: &envoy_type_v3.TokenBucket{
			MaxTokens: uint32(i),
			FillInterval: &duration.Duration{
				Seconds: des.Action.FillInterval.Seconds,
				Nanos:   des.Action.FillInterval.Nanos,
			},
		},
		FilterEnabled: &envoy_config_core_v3.RuntimeFractionalPercent{
			RuntimeKey: util.Struct_EnvoyLocalRateLimit_Enabled,
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
		FilterEnforced: &envoy_config_core_v3.RuntimeFractionalPercent{
			RuntimeKey: util.Struct_EnvoyLocalRateLimit_Enforced,
			DefaultValue: &envoy_type_v3.FractionalPercent{
				Numerator:   100,
				Denominator: envoy_type_v3.FractionalPercent_HUNDRED,
			},
		},
	}
	t, err := util.MessageToStruct(envoyLocDes)
	if err == nil {
		patch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
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
							Kind: &structpb.Value_StringValue{StringValue: util.Envoy_LocalRateLimit},
						},
						util.Struct_HttpFilter_TypedConfig: {
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										util.Struct_Any_AtType: {
											Kind: &structpb.Value_StringValue{StringValue: util.TypeUrl_UdpaTypedStruct},
										},
										util.Struct_Any_TypedUrl: {
											Kind: &structpb.Value_StringValue{StringValue: util.TypeUrl_EnvoyLocalRatelimit},
										},
										util.Struct_Any_Value: {
											Kind: &structpb.Value_StructValue{StructValue: t},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		ef.ConfigPatches = append(ef.ConfigPatches, patch)
	}
	return ef
}
