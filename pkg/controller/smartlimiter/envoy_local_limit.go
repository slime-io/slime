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
	"yun.netease.com/slime/pkg/apis/networking/v1alpha3"
	"yun.netease.com/slime/pkg/util"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_filters_http_local_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	structpb "github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/duration"
	networking "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *ReconcileSmartLimiter) GenerateEnvoyLocalLimit(rateLimitConf microservicev1alpha1.SmartLimiterSpec,
	material map[string]string, instance *microservicev1alpha1.SmartLimiter) (
	*v1alpha3.EnvoyFilter, []*microservicev1alpha1.SmartLimitDescriptor) {
	descriptor := make([]*microservicev1alpha1.SmartLimitDescriptor, 0, len(rateLimitConf.Descriptors))
	for _, des := range rateLimitConf.Descriptors {
		podDes := &microservicev1alpha1.SmartLimitDescriptor{}
		if shouldUpdate, _ := util.CalculateTemplateBool(des.Condition, material); shouldUpdate {
			if des.Action != nil {
				if rateLimitValue, err := util.CalculateTemplate(des.Action.Quota, material); err == nil {
					podDes.Action = &microservicev1alpha1.Action{
						Quota:        fmt.Sprintf("%d", rateLimitValue),
						FillInterval: des.Action.FillInterval,
					}
				}
				descriptor = append(descriptor, podDes)
			}
		}
	}
	loc := types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}
	svc := &v1.Service{}
	_ = r.client.Get(context.TODO(), loc, svc)
	ef := &networking.EnvoyFilter{
		WorkloadSelector: &networking.WorkloadSelector{
			Labels: svc.Spec.Selector,
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
		if mi, err := util.ProtoToMap(ef); err == nil {
			x := &v1alpha3.EnvoyFilter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instance.Name + "." + instance.Namespace + "." + "local-ratelimit",
					Namespace: instance.Namespace,
				},
				Spec: mi,
			}
			return x, descriptor
		}
	}
	return nil, nil
}
