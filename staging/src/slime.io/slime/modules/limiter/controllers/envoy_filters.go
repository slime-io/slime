package controllers

import (
	"context"
	"fmt"

	networking "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/util"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

// LimiterSpec info come from config and SmartLimiterSpec
type LimiterSpec struct {
	rls                         string
	gw                          bool
	labels                      map[string]string
	loc                         types.NamespacedName
	disableGlobalRateLimit      bool
	disableInsertLocalRateLimit bool
	context                     string
}

func (r *SmartLimiterReconciler) GenerateEnvoyConfigs(spec microservicev1alpha2.SmartLimiterSpec,
	material map[string]string, loc types.NamespacedName) (
	map[string]*networking.EnvoyFilter, map[string]*microservicev1alpha2.SmartLimitDescriptors, []*model.Descriptor, error,
) {
	materialInterface := util.MapToMapInterface(material)
	setsEnvoyFilter := make(map[string]*networking.EnvoyFilter)
	setsSmartLimitDescriptor := make(map[string]*microservicev1alpha2.SmartLimitDescriptors)
	globalDescriptors := make([]*model.Descriptor, 0)
	params := &LimiterSpec{
		rls:                         spec.Rls,
		gw:                          spec.Gateway,
		labels:                      spec.WorkloadSelector,
		loc:                         loc,
		disableGlobalRateLimit:      r.cfg.GetDisableGlobalRateLimit(),
		disableInsertLocalRateLimit: r.cfg.GetDisableInsertLocalRateLimit(),
	}

	var sets []*networking.Subset
	if !params.gw {
		host := util.UnityHost(loc.Name, loc.Namespace)
		if controllers.HostSubsetMapping.Get(host) != nil {
			sets = controllers.HostSubsetMapping.Get(host).([]*networking.Subset)
		} else {
			sets = make([]*networking.Subset, 0, 1)
		}
	}

	sets = append(sets, &networking.Subset{Name: util.Wellkonw_BaseSet})
	svcSelector := generateServiceSelector(r, params)
	if len(svcSelector) == 0 {
		log.Info("get empty svc selector by %v", params)
		return setsEnvoyFilter, setsSmartLimitDescriptor, globalDescriptors, nil
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
							// log.Infof("after calculate, the quota %s is %d",des.Action.Quota,rateLimitValue)
							validDescriptor.Descriptor_ = append(validDescriptor.Descriptor_, &microservicev1alpha2.SmartLimitDescriptor{
								Action: &microservicev1alpha2.SmartLimitDescriptor_Action{
									Quota:        fmt.Sprintf("%d", rateLimitValue),
									FillInterval: des.Action.FillInterval,
									Strategy:     des.Action.Strategy,
								},
								Match:  des.Match,
								Target: des.Target,
							})
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

func descriptorsToEnvoyFilter(descriptors []*microservicev1alpha2.SmartLimitDescriptor, labels map[string]string, params *LimiterSpec) *networking.EnvoyFilter {
	ef := &networking.EnvoyFilter{
		WorkloadSelector: &networking.WorkloadSelector{
			Labels: labels,
		},
	}
	ef.ConfigPatches = make([]*networking.EnvoyFilter_EnvoyConfigObjectPatch, 0)
	globalDescriptors := make([]*microservicev1alpha2.SmartLimitDescriptor, 0)
	localDescriptors := make([]*microservicev1alpha2.SmartLimitDescriptor, 0)

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
		server := getRateLimiterServerCluster(params.rls)
		httpFilterEnvoyRateLimitPatch := generateEnvoyHttpFilterGlobalRateLimitPatch(server)
		if httpFilterEnvoyRateLimitPatch != nil {
			ef.ConfigPatches = append(ef.ConfigPatches, httpFilterEnvoyRateLimitPatch)
		}
	}

	// enable and config plugin envoy.filters.http.local_ratelimit
	if len(localDescriptors) > 0 {
		// if disableInsertLocalRateLimit=false in gw, multi smartlimiter will patch duplicate configurations, This will lead dysfunctional behavior.
		// so disable it, and apply envoyfilter like staging/src/slime.io/slime/modules/limiter/install/gw_limiter_envoyfilter.yaml to your gw pilot
		if !params.disableInsertLocalRateLimit {
			httpFilterLocalRateLimitPatch := generateHttpFilterLocalRateLimitPatch(params.context)
			ef.ConfigPatches = append(ef.ConfigPatches, httpFilterLocalRateLimitPatch)
		} else {
			log.Infof("disableInsertLocalRateLimit set true, skip")
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

// TODO get labels base on serviceentry
func generateServiceSelector(r *SmartLimiterReconciler, spec *LimiterSpec) map[string]string {
	var labels map[string]string
	// top level
	if len(spec.labels) > 0 {
		labels = spec.labels
	} else {
		svc := &v1.Service{}
		if err := r.Client.Get(context.TODO(), spec.loc, svc); err != nil {
			if errors.IsNotFound(err) {
				log.Errorf("svc %s is not found", spec.loc)
			} else {
				log.Errorf("get svc %s err: %+v", spec.loc, err.Error())
			}
		}
		labels = svc.Spec.Selector
	}
	return labels
}
