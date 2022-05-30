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

func (r *SmartLimiterReconciler) GenerateEnvoyConfigs(spec microservicev1alpha2.SmartLimiterSpec,
	material map[string]string, loc types.NamespacedName) (
	map[string]*networking.EnvoyFilter, map[string]*microservicev1alpha2.SmartLimitDescriptors, []*model.Descriptor, error,
) {
	materialInterface := util.MapToMapInterface(material)
	setsEnvoyFilter := make(map[string]*networking.EnvoyFilter)
	setsSmartLimitDescriptor := make(map[string]*microservicev1alpha2.SmartLimitDescriptors)
	// global descriptors
	globalDescriptors := make([]*model.Descriptor, 0)
	host := util.UnityHost(loc.Name, loc.Namespace)
	rls := spec.Rls

	// get destinationrule subset of the host
	var sets []*networking.Subset
	if controllers.HostSubsetMapping.Get(host) != nil {
		sets = controllers.HostSubsetMapping.Get(host).([]*networking.Subset)
	} else {
		sets = make([]*networking.Subset, 0, 1)
	}
	sets = append(sets, &networking.Subset{Name: util.Wellkonw_BaseSet})

	svc := &v1.Service{}
	if err := r.Client.Get(context.TODO(), loc, svc); err != nil {
		if errors.IsNotFound(err) {
			log.Errorf("svc %s:%s is not found", loc.Name, loc.Namespace)
		} else {
			log.Errorf("get svc %s:%s err: %+v", loc.Name, loc.Namespace, err.Error())
		}
		return setsEnvoyFilter, setsSmartLimitDescriptor, globalDescriptors, err
	}
	svcSelector := svc.Spec.Selector

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
				ef := descriptorsToEnvoyFilter(validDescriptor.Descriptor_, selector, loc, rls)
				setsEnvoyFilter[set.Name] = ef
				setsSmartLimitDescriptor[set.Name] = validDescriptor

				desc := descriptorsToGlobalRateLimit(validDescriptor.Descriptor_, loc)
				globalDescriptors = append(globalDescriptors, desc...)
			}
		}
	}
	return setsEnvoyFilter, setsSmartLimitDescriptor, globalDescriptors, nil
}

func descriptorsToEnvoyFilter(descriptors []*microservicev1alpha2.SmartLimitDescriptor, labels map[string]string, loc types.NamespacedName, rls string) *networking.EnvoyFilter {
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
	httpRouterPatches, err := generateHttpRouterPatch(descriptors, loc)
	if err != nil {
		log.Errorf("generateHttpRouterPatch err: %+v", err.Error())
		return nil
	} else if len(httpRouterPatches) > 0 {
		ef.ConfigPatches = append(ef.ConfigPatches, httpRouterPatches...)
	}

	// config plugin envoy.filters.http.ratelimit
	if len(globalDescriptors) > 0 {
		server := getRateLimiterServerCluster(rls)
		httpFilterEnvoyRateLimitPatch := generateEnvoyHttpFilterGlobalRateLimitPatch(server)
		if httpFilterEnvoyRateLimitPatch != nil {
			ef.ConfigPatches = append(ef.ConfigPatches, httpFilterEnvoyRateLimitPatch)
		}
	}

	// enable and config plugin envoy.filters.http.local_ratelimit
	if len(localDescriptors) > 0 {
		httpFilterLocalRateLimitPatch := generateHttpFilterLocalRateLimitPatch()
		ef.ConfigPatches = append(ef.ConfigPatches, httpFilterLocalRateLimitPatch)

		perFilterPatch := generateLocalRateLimitPerFilterPatch(localDescriptors, loc)
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
