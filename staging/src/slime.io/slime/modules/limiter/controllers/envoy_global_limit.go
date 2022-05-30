package controllers

import (
	"fmt"
	"strconv"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	structpb "github.com/gogo/protobuf/types"
	networking "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/types"
	"slime.io/slime/framework/util"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

func generateEnvoyHttpFilterGlobalRateLimitPatch(server string) *networking.EnvoyFilter_EnvoyConfigObjectPatch {
	rateLimitServiceConfig := generateRateLimitService(server)
	rs, err := util.MessageToStruct(rateLimitServiceConfig)
	if err != nil {
		log.Errorf("MessageToStruct err: %+v", err.Error())
		return nil
	}
	patch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
		Match:   generateEnvoyHttpFilterMatch(),
		Patch:   generateEnvoyHttpFilterRateLimitServicePatch(rs),
	}
	return patch
}

func generateRateLimitService(clusterName string) *envoy_config_ratelimit_v3.RateLimitServiceConfig {
	rateLimitServiceConfig := &envoy_config_ratelimit_v3.RateLimitServiceConfig{
		GrpcService: &envoy_core_v3.GrpcService{
			TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
				ClusterName: clusterName,
			}},
		},
		TransportApiVersion: envoy_core_v3.ApiVersion_V3,
	}
	return rateLimitServiceConfig
}

func generateEnvoyHttpFilterMatch() *networking.EnvoyFilter_EnvoyConfigObjectMatch {
	return &networking.EnvoyFilter_EnvoyConfigObjectMatch{
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
	}
}

func generateEnvoyHttpFilterRateLimitServicePatch(rs *structpb.Struct) *networking.EnvoyFilter_Patch {
	return &networking.EnvoyFilter_Patch{
		Operation: networking.EnvoyFilter_Patch_INSERT_BEFORE,
		Value: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				util.Struct_HttpFilter_Name: {
					Kind: &structpb.Value_StringValue{StringValue: model.EnvoyFiltersHttpRateLimit},
				},
				util.Struct_HttpFilter_TypedConfig: {
					Kind: &structpb.Value_StructValue{
						StructValue: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								util.Struct_Any_AtType: {
									Kind: &structpb.Value_StringValue{StringValue: util.TypeUrl_UdpaTypedStruct},
								},
								util.Struct_Any_TypedUrl: {
									Kind: &structpb.Value_StringValue{StringValue: model.TypeUrlEnvoyRateLimit},
								},
								util.Struct_Any_Value: {
									Kind: &structpb.Value_StructValue{
										StructValue: &structpb.Struct{
											Fields: map[string]*structpb.Value{
												model.StructDomain: {
													Kind: &structpb.Value_StringValue{StringValue: model.Domain},
												},
												model.StructRateLimitService: {
													Kind: &structpb.Value_StructValue{StructValue: rs},
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

func generateGlobalRateLimitDescriptor(descriptors []*microservicev1alpha2.SmartLimitDescriptor, loc types.NamespacedName) []*model.Descriptor {
	desc := make([]*model.Descriptor, 0)
	for _, descriptor := range descriptors {
		quota, unit, err := calculateQuotaPerUnit(descriptor)
		if err != nil {
			log.Errorf("calculateQuotaPerUnit err: %+v", err)
			return desc
		}
		item := &model.Descriptor{
			Value: generateDescriptorValue(descriptor, loc),
			RateLimit: &model.RateLimit{
				RequestsPerUnit: uint32(quota),
				Unit:            unit,
			},
		}
		if len(descriptor.Match) == 0 {
			item.Key = model.GenericKey
		} else {
			item.Key = model.HeaderValueMatch
		}
		desc = append(desc, item)
	}
	return desc
}

// https://github.com/envoyproxy/ratelimit only support per second, minute, hour, and day limits
func calculateQuotaPerUnit(descriptor *microservicev1alpha2.SmartLimitDescriptor) (quota int, unit string, err error) {
	quota, err = strconv.Atoi(descriptor.Action.Quota)
	if err != nil {
		return quota, unit, err
	}
	seconds := descriptor.Action.FillInterval.Seconds
	switch seconds {
	case 60 * 60 * 24:
		unit = "DAY"
	case 60 * 60:
		unit = "HOUR"
	case 60:
		unit = "MINUTE"
	case 1:
		unit = "SECOND"
	default:
		return quota, unit, fmt.Errorf("invalid time in global rate limit")
	}
	return quota, unit, nil
}

func getRateLimiterServerCluster(server string) string {
	if server == "" {
		return model.RateLimitService
	} else {
		return server
	}
}

func getConfigMapNamespaceName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: model.ConfigMapNamespace,
		Name:      model.ConfigMapName,
	}
}
