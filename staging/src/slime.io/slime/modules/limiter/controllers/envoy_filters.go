package controllers

import (
	"context"
	"fmt"
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"slime.io/slime/framework/bootstrap/resource"
	slime_serviceregistry "slime.io/slime/framework/bootstrap/serviceregistry/model"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/util"
	"slime.io/slime/modules/limiter/api/config"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

// LimiterSpec info come from config and SmartLimiterSpec
type LimiterSpec struct {
	rls                          *config.RateLimitService
	gw                           bool
	labels                       map[string]string
	loc                          types.NamespacedName
	disableGlobalRateLimit       bool
	disableInsertLocalRateLimit  bool
	disableInsertGlobalRateLimit bool
	context                      string
	rlsNs                        string
	// svc hostname
	host   string
	target *microservicev1alpha2.Target

	domain       string
	rlsConfigmap *config.RlsConfigMap

	proxyVersion string
}

func (r *SmartLimiterReconciler) GenerateEnvoyConfigs(spec microservicev1alpha2.SmartLimiterSpec,
	material map[string]string, loc types.NamespacedName) (
	map[string]*networkingapi.EnvoyFilter, map[string]*microservicev1alpha2.SmartLimitDescriptors, []*model.Descriptor, error,
) {
	materialInterface := util.MapToMapInterface(material)
	setsEnvoyFilter := make(map[string]*networkingapi.EnvoyFilter)
	setsSmartLimitDescriptor := make(map[string]*microservicev1alpha2.SmartLimitDescriptors)
	globalDescriptors := make([]*model.Descriptor, 0)
	params := &LimiterSpec{
		rls:                          r.cfg.GetRls(),
		gw:                           spec.Gateway,
		labels:                       spec.WorkloadSelector,
		loc:                          loc,
		disableGlobalRateLimit:       r.cfg.GetDisableGlobalRateLimit(),
		disableInsertLocalRateLimit:  r.cfg.GetDisableInsertLocalRateLimit(),
		disableInsertGlobalRateLimit: r.cfg.GetDisableInsertGlobalRateLimit(),
		host:                         spec.Host,
		target:                       spec.Target,
		domain:                       r.cfg.GetDomain(),
		rlsConfigmap:                 r.cfg.GetRlsConfigMap(),
		proxyVersion:                 r.cfg.GetProxyVersion(),
	}

	var sets []*networkingapi.Subset

	meta, ok := r.interest.Get(FQN(loc.Namespace, loc.Name))
	if !ok {
		return setsEnvoyFilter, setsSmartLimitDescriptor, globalDescriptors, nil
	}

	// subset is only queried when there is a `service` in inbound
	if !meta.sidecarOutbound && !meta.gateway && len(meta.workloadSelector) == 0 {
		host := meta.host
		if meta.seHost != "" {
			host = meta.seHost
		}
		if got := controllers.HostSubsetMapping.Get(host); len(got) != 0 {
			sets = got
		}
	}

	sets = append(sets, &networkingapi.Subset{Name: util.WellknownBaseSet})

	svcSelector, err := generateServiceSelector(r, params)
	if err != nil {
		log.Errorf("get svc selector err base on %v", params)
		return setsEnvoyFilter, setsSmartLimitDescriptor, globalDescriptors, err
	}

	for _, set := range sets {
		if setDescriptor, ok := spec.Sets[set.Name]; !ok {
			// sets is specified in the descriptor, but not found in the Destinationrule set
			// the set must be deleted in the DestinationRule set
			setsEnvoyFilter[set.Name] = nil
		} else {
			validDescriptor := &microservicev1alpha2.SmartLimitDescriptors{}
			for _, des := range setDescriptor.Descriptor_ {
				// update the EnvoyFilter when condition value is true after calculate
				if shouldUpdate, err := util.CalculateTemplateBool(des.Condition, materialInterface); err != nil {
					log.Errorf("calaulate %s condition err, %+v", des.Condition, err.Error())
					continue
				} else if !shouldUpdate {
					log.Infof("the value of condition %s is false", des.Condition)
				} else {
					// update
					if des.Action != nil {
						if rateLimitValue, err := util.CalculateTemplate(des.Action.Quota, materialInterface); err != nil {
							log.Errorf("calculate quota %s err, %+v", des.Action.Quota, err.Error())
						} else {
							ips := exactIPs(des)
							if ips == nil {
								sd := warpDescriptors(des, rateLimitValue)
								validDescriptor.Descriptor_ = append(validDescriptor.Descriptor_, sd)
							} else {
								for _, ip := range ips {
									sd := warpDescriptors(des, rateLimitValue)
									for i := range sd.Match {
										if sd.Match[i].MatchSource == microservicev1alpha2.SmartLimitDescriptor_Matcher_SourceIpMatch {
											sd.Match[i].ExactMatch = ip
											break
										}
									}
									validDescriptor.Descriptor_ = append(validDescriptor.Descriptor_, sd)
								}
							}
						}
					}
				}
			}

			if len(validDescriptor.Descriptor_) == 0 {
				log.Infof("not matchd descriptor in %s", set.Name)
				setsEnvoyFilter[set.Name] = nil
			} else {
				// prepare to generate ef according the descriptor in validDescriptor
				selector := util.CopyMap(svcSelector)
				for k, v := range set.Labels {
					selector[k] = v
				}
				ef := descriptorsToEnvoyFilter(validDescriptor.Descriptor_, selector, params)
				setsEnvoyFilter[set.Name] = ef
				setsSmartLimitDescriptor[set.Name] = validDescriptor

				desc := descriptorsToGlobalRateLimit(validDescriptor.Descriptor_, params.loc)
				globalDescriptors = append(globalDescriptors, desc...)
			}
		}
	}
	return setsEnvoyFilter, setsSmartLimitDescriptor, globalDescriptors, nil
}

func warpDescriptors(descriptor *microservicev1alpha2.SmartLimitDescriptor, rateLimitValue int) *microservicev1alpha2.SmartLimitDescriptor {
	des := descriptor.DeepCopy()
	sd := &microservicev1alpha2.SmartLimitDescriptor{
		Action: &microservicev1alpha2.SmartLimitDescriptor_Action{
			Quota:        fmt.Sprintf("%d", rateLimitValue),
			FillInterval: des.Action.FillInterval,
			Strategy:     des.Action.Strategy,
		},
		Match:  des.Match,
		Target: des.Target,
	}
	headers := generateHeadersToAdd(des)
	if len(headers) > 0 {
		sd.Action.HeadersToAdd = headers
	}
	return sd
}

func exactIPs(des *microservicev1alpha2.SmartLimitDescriptor) []string {
	for _, m := range des.Match {
		if m.MatchSource == microservicev1alpha2.SmartLimitDescriptor_Matcher_SourceIpMatch {
			return strings.Split(m.ExactMatch, ",")
		}
	}
	return nil
}

func descriptorsToEnvoyFilter(descriptors []*microservicev1alpha2.SmartLimitDescriptor, labels map[string]string, params *LimiterSpec) *networkingapi.EnvoyFilter {
	ef := &networkingapi.EnvoyFilter{}
	ef.ConfigPatches = make([]*networkingapi.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	globalDescriptors := make([]*microservicev1alpha2.SmartLimitDescriptor, 0)
	localDescriptors := make([]*microservicev1alpha2.SmartLimitDescriptor, 0)

	if len(labels) > 0 {
		ef.WorkloadSelector = &networkingapi.WorkloadSelector{Labels: labels}
	}

	// split descriptors due to different envoy plugins
	for _, descriptor := range descriptors {
		if descriptor.Action != nil {
			if descriptor.Action.Strategy == model.GlobalSmartLimiter {
				globalDescriptors = append(globalDescriptors, descriptor)
			} else {
				localDescriptors = append(localDescriptors, descriptor)
			}
		}
	}

	// http router
	httpRouterPatches, err := generateHttpRouterPatch(descriptors, params)
	if err != nil {
		log.Errorf("generateHttpRouterPatch err: %+v", err.Error())
		return nil
	} else if len(httpRouterPatches) > 0 {
		ef.ConfigPatches = append(ef.ConfigPatches, httpRouterPatches...)
	}

	// config plugin envoy.filters.http.ratelimit
	if len(globalDescriptors) > 0 {
		// disable insert envoy.filters.http.ratelimit before route
		if !params.disableInsertGlobalRateLimit {
			server, err := getRateLimiterService(params.rls)
			if err != nil {
				log.Errorf("getRateLimiterService err: %s, skip", err.Error())
			}
			domain := getDomain(params.domain)
			httpFilterEnvoyRateLimitPatch := generateEnvoyHttpFilterGlobalRateLimitPatch(params.context, server, domain, params.proxyVersion)
			if httpFilterEnvoyRateLimitPatch != nil {
				ef.ConfigPatches = append(ef.ConfigPatches, httpFilterEnvoyRateLimitPatch)
			}
		} else {
			log.Debugf("disableGlobalRateLimit set true, skip")
		}
	}

	// enable and config plugin envoy.filters.http.local_ratelimit
	if len(localDescriptors) > 0 {
		// if disableInsertLocalRateLimit=false in gw, multi smartlimiter will patch duplicate configurations, This will lead dysfunctional behavior.
		// so disable it, and apply envoyfilter like staging/src/slime.io/slime/modules/limiter/install/gw_limiter_envoyfilter.yaml to your gw pilot
		if !params.disableInsertLocalRateLimit {
			httpFilterLocalRateLimitPatch := generateHttpFilterLocalRateLimitPatch(params.context, params.proxyVersion)
			ef.ConfigPatches = append(ef.ConfigPatches, httpFilterLocalRateLimitPatch)
		} else {
			log.Debugf("disableInsertLocalRateLimit set true, skip")
		}
		perFilterPatch := generateLocalRateLimitPerFilterPatch(localDescriptors, params)
		ef.ConfigPatches = append(ef.ConfigPatches, perFilterPatch...)
	}
	return ef
}

func descriptorsToGlobalRateLimit(descriptors []*microservicev1alpha2.SmartLimitDescriptor, loc types.NamespacedName) []*model.Descriptor {
	globalDescriptors := make([]*microservicev1alpha2.SmartLimitDescriptor, 0)
	for _, descriptor := range descriptors {
		if descriptor.Action.Strategy == model.GlobalSmartLimiter {
			globalDescriptors = append(globalDescriptors, descriptor)
		}
	}
	return generateGlobalRateLimitDescriptor(globalDescriptors, loc)
}

func generateServiceSelector(r *SmartLimiterReconciler, spec *LimiterSpec) (map[string]string, error) {
	// service selector
	var labels map[string]string
	var err error
	var istioSvc *slime_serviceregistry.Service

	if spec.gw || len(spec.labels) > 0 {
		return spec.labels, nil
	}

	// gen workload selector for mesh service
	if spec.host != "" {
		istioSvc, err = getIstioService(r, types.NamespacedName{Namespace: spec.loc.Namespace, Name: spec.host})
		if err == nil {
			labels = formatLabels(getIstioServiceLabels(istioSvc))
		}
	} else {
		labels, err = getK8sServiceLabels(r, spec)
	}
	return labels, err
}

// get svc labels base on framework
func getIstioServiceLabels(svc *slime_serviceregistry.Service) map[string]string {
	selector := svc.Attributes.LabelSelectors
	log.Debugf("get istio service labelselector %s", selector)
	return selector
}

func getIstioService(r *SmartLimiterReconciler, nn types.NamespacedName) (*slime_serviceregistry.Service, error) {
	// get config from framework
	config := r.env.ConfigController.Get(resource.IstioService, nn.Name, nn.Namespace)
	if config == nil {
		return nil, fmt.Errorf("get empty config base on %s/%s", nn.Namespace, nn.Name)
	}

	svc, ok := config.Spec.(*slime_serviceregistry.Service)
	if !ok {
		return nil, fmt.Errorf("convert config to istio service err")
	}
	return svc, nil
}

// get svc labels base on clientSet ,
// if the svc arrives later than smartlimiter,
// ticker mechanism will ensure modules get the svc
func getK8sServiceLabels(r *SmartLimiterReconciler, spec *LimiterSpec) (map[string]string, error) {
	svc := &v1.Service{}

	err := r.Client.Get(context.TODO(), spec.loc, svc)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("k8s svc %s is not found", spec.loc)
		} else {
			return nil, fmt.Errorf("get k8s svc %s err: %+v", spec.loc, err.Error())
		}
	}
	return svc.Spec.Selector, nil
}

func formatLabels(selector map[string]string) map[string]string {
	labels := make(map[string]string)
	for key, val := range selector {
		if val == "" {
			labels[key] = "default"
		} else {
			labels[key] = val
		}
	}
	return labels
}

func generateHeadersToAdd(des *microservicev1alpha2.SmartLimitDescriptor) []*microservicev1alpha2.Header {
	headers := make([]*microservicev1alpha2.Header, 0)
	for _, item := range des.Action.HeadersToAdd {
		headers = append(headers, &microservicev1alpha2.Header{
			Key:   item.GetKey(),
			Value: item.GetValue(),
		})
	}
	return headers
}
