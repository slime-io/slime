package controllers

import (
	"context"
	"fmt"
	"hash/adler32"
	"reflect"
	"sort"
	"strconv"
	"strings"

	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_ratelimit_v3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	"google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v2"
	networking "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"slime.io/slime/framework/util"
	"slime.io/slime/modules/limiter/api/config"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

func generateEnvoyHttpFilterGlobalRateLimitPatch(context, server, domain, proxyVersion string) *networking.EnvoyFilter_EnvoyConfigObjectPatch {
	rateLimitServiceConfig := generateRateLimitService(server)
	rs, err := util.MessageToStruct(rateLimitServiceConfig)
	if err != nil {
		log.Errorf("MessageToStruct err: %+v", err.Error())
		return nil
	}
	patch := &networking.EnvoyFilter_EnvoyConfigObjectPatch{
		ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
		Match:   generateEnvoyHttpFilterMatch(context, proxyVersion),
		Patch:   generateEnvoyHttpFilterRateLimitServicePatch(rs, domain),
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

func generateEnvoyHttpFilterMatch(context string, proxyVersion string) *networking.EnvoyFilter_EnvoyConfigObjectMatch {
	match := &networking.EnvoyFilter_EnvoyConfigObjectMatch{
		Context: networking.EnvoyFilter_SIDECAR_INBOUND,
		ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
			Listener: &networking.EnvoyFilter_ListenerMatch{
				FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
					Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
						Name: util.EnvoyHTTPConnectionManager,
						SubFilter: &networking.EnvoyFilter_ListenerMatch_SubFilterMatch{
							Name: util.EnvoyHTTPRouter,
						},
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

	if context == model.Gateway {
		match.Context = networking.EnvoyFilter_GATEWAY
	} else if context == model.Outbound {
		match.Context = networking.EnvoyFilter_SIDECAR_OUTBOUND
	}
	return match
}

func generateEnvoyHttpFilterRateLimitServicePatch(rs *structpb.Struct, domain string) *networking.EnvoyFilter_Patch {
	return &networking.EnvoyFilter_Patch{
		Operation: networking.EnvoyFilter_Patch_INSERT_BEFORE,
		Value: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				util.StructHttpFilterName: {
					Kind: &structpb.Value_StringValue{StringValue: model.EnvoyFiltersHttpRateLimit},
				},
				util.StructHttpFilterTypedConfig: {
					Kind: &structpb.Value_StructValue{
						StructValue: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								util.StructAnyAtType: {
									Kind: &structpb.Value_StringValue{StringValue: util.TypeURLUDPATypedStruct},
								},
								util.StructAnyTypeURL: {
									Kind: &structpb.Value_StringValue{StringValue: model.TypeUrlEnvoyRateLimit},
								},
								util.StructAnyValue: {
									Kind: &structpb.Value_StructValue{
										StructValue: &structpb.Struct{
											Fields: map[string]*structpb.Value{
												model.StructDomain: {
													Kind: &structpb.Value_StringValue{StringValue: domain},
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
		ratelimit := &model.RateLimit{
			RequestsPerUnit: uint32(quota),
			Unit:            unit,
		}

		if len(descriptor.Match) == 0 {
			item := &model.Descriptor{}
			item.Key = model.GenericKey
			item.RateLimit = ratelimit
			item.Value = generateDescriptorValue(descriptor, loc)
			desc = append(desc, item)

		} else if containsHeaderRequest(descriptors) {
			item := &model.Descriptor{}
			item.Key = model.GenericKey
			item.Value = generateDescriptorValue(descriptor, loc)
			item.Descriptors = append(item.Descriptors, model.Descriptor{
				Key:       generateDescriptorKey(descriptor, loc),
				RateLimit: ratelimit,
			})
			desc = append(desc, item)
		} else {
			var useQuery, useHeader, useSourceIP bool
			for _, match := range descriptor.Match {
				switch match.MatchSource {
				case microservicev1alpha2.SmartLimitDescriptor_Matcher_SourceIpMatch:
					useSourceIP = true
				case microservicev1alpha2.SmartLimitDescriptor_Matcher_QueryMatch:
					useQuery = true
				case microservicev1alpha2.SmartLimitDescriptor_Matcher_HeadMatch:
					// compatible with old version
					if match.UseQueryMatch {
						useQuery = true
					} else {
						useHeader = true
					}
				default:
					useHeader = true
				}
			}
			item := generateDescriptor(descriptor, useQuery, useHeader, useSourceIP, loc, ratelimit)
			desc = append(desc, item)
		}
	}
	return desc
}

func generateDescriptor(descriptor *microservicev1alpha2.SmartLimitDescriptor, useQuery, useHeader,
	useSourceIP bool, loc types.NamespacedName, ratelimit *model.RateLimit,
) *model.Descriptor {
	desc := model.Descriptor{}
	var sourceIPDesc, headerDesc, queryDesc model.Descriptor

	val := generateDescriptorValue(descriptor, loc)

	// suquence:  query > header > sourceIp
	if useSourceIP {
		desc = createDescriptor(model.RemoteAddress, generateRemoteAddressDescriptorValue(descriptor), ratelimit)
		sourceIPDesc = createDescriptor(model.GenericKey, val, nil)
		sourceIPDesc.Descriptors = []model.Descriptor{desc}
	}

	if useHeader {
		headerDesc = createDescriptor(model.HeaderValueMatch, val, nil)
		if useSourceIP {
			headerDesc.Descriptors = []model.Descriptor{sourceIPDesc}
		} else {
			headerDesc.RateLimit = ratelimit
		}
	}

	if useQuery {
		queryDesc = createDescriptor(model.QueryMatch, val, nil)
		if useHeader {
			queryDesc.Descriptors = []model.Descriptor{headerDesc}
		} else if useSourceIP {
			queryDesc.Descriptors = []model.Descriptor{sourceIPDesc}
		} else {
			queryDesc.RateLimit = ratelimit
		}
	}

	if useQuery {
		desc = queryDesc
	} else if useHeader {
		desc = headerDesc
	} else {
		desc = sourceIPDesc
	}
	return &desc
}

func createDescriptor(key, value string, ratelimit *model.RateLimit) model.Descriptor {
	desc := model.Descriptor{}
	desc.Key = key
	desc.Value = value
	desc.RateLimit = ratelimit
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

func getRateLimiterService(service *config.RateLimitService) (string, error) {
	var svc string
	var port int32
	if service != nil {
		svc = service.GetService()
		port = service.GetPort()
		if svc == "" {
			return "", fmt.Errorf("rls svc is empty")
		}
		if port == 0 {
			return "", fmt.Errorf("rls svc port is zero")
		}
		return fmt.Sprintf("outbound|%d||%s", port, svc), nil
	}
	return "", fmt.Errorf("rls svc is empty")
}

func getDomain(str string) string {
	if str != "" {
		return str
	}
	return model.Domain
}

func getConfigMapNamespaceName(cm *config.RlsConfigMap) (types.NamespacedName, error) {
	loc := types.NamespacedName{
		Namespace: cm.Namespace,
		Name:      cm.Name,
	}
	if loc.Namespace == "" || loc.Name == "" {
		return loc, fmt.Errorf("rlsConfigMap is invalid")
	}
	return loc, nil
}

// if configmap rate-limit-config not exist, return
func refreshConfigMap(desc []*model.Descriptor, r *SmartLimiterReconciler, serviceLoc types.NamespacedName) {
	loc, err := getConfigMapNamespaceName(r.cfg.RlsConfigMap)
	if err != nil {
		log.Errorf("getConfigMapNamespaceName err: %s", err.Error())
		return
	}

	found := &v1.ConfigMap{}
	err = r.Client.Get(context.TODO(), loc, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Errorf("configmap %s:%s is not found", loc.Namespace, loc.Name)
			return
		} else {
			log.Errorf("get configmap %s:%s err: %+v", loc.Namespace, loc.Name, err.Error())
			return
		}
	}

	config, ok := found.Data[model.ConfigMapConfig]
	if !ok {
		log.Errorf("config.yaml not found in configmap %s:%s", loc.Namespace, loc.Name)
		return
	}
	rc := &model.RateLimitConfig{}
	if err = yaml.Unmarshal([]byte(config), &rc); err != nil {
		log.Errorf("unmarshal ratelimitConfig %s err: %+v", config, err.Error())
		return
	}
	newCm := make([]*model.Descriptor, 0)
	serviceInfo := fmt.Sprintf("%s.%s", serviceLoc.Name, serviceLoc.Namespace)
	for _, item := range rc.Descriptors {
		if !strings.Contains(item.Value, serviceInfo) {
			newCm = append(newCm, item)
		}
	}
	newCm = append(newCm, desc...)
	sort.Slice(newCm, func(i, j int) bool {
		return newCm[i].Value < newCm[j].Value
	})

	configmap := constructConfigMap(newCm, loc, getDomain(r.cfg.GetDomain()))
	if !reflect.DeepEqual(found.Data["config.yaml"], configmap.Data["config.yaml"]) {
		log.Infof("update rate-limit-config %s:%s", loc.Namespace, loc.Name)
		configmap.ResourceVersion = found.ResourceVersion
		err = r.Client.Update(context.TODO(), configmap)
		if err != nil {
			log.Infof("update configmap %s:%s err: %+v", loc.Namespace, loc.Name, err.Error())
			return
		}
	}
}

func constructConfigMap(desc []*model.Descriptor, loc types.NamespacedName, domain string) *v1.ConfigMap {
	rateLimitConfig := &model.RateLimitConfig{
		Domain:      domain,
		Descriptors: desc,
	}

	b, _ := yaml.Marshal(rateLimitConfig)
	configmap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      loc.Name,
			Namespace: loc.Namespace,
		},
		Data: map[string]string{
			model.ConfigMapConfig: string(b),
		},
	}
	return configmap
}

func generateDescriptorKey(item *microservicev1alpha2.SmartLimitDescriptor, loc types.NamespacedName) string {
	id := adler32.Checksum([]byte(item.String() + loc.String()))
	return fmt.Sprintf("RequestHeader[%s.%s]-Id[%d]", loc.Name, loc.Namespace, id)
}

func containsHeaderRequest(descriptors []*microservicev1alpha2.SmartLimitDescriptor) bool {
	for _, descriptor := range descriptors {
		for _, match := range descriptor.Match {
			if match.PresentMatchSeparate {
				return true
			}
		}
	}
	return false
}
