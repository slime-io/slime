package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

func ConvertRegistrySourceToArgs(in *RegistrySource, out *bootstrap.RegistryArgs) error {
	if out == nil {
		return fmt.Errorf("the input RegistryArgs must not be nil")
	}
	if in.Spec.Zookeeper != nil {
		out.ZookeeperSource = &bootstrap.ZookeeperSourceArgs{}
		convertZookeeperSource(in.Spec.Zookeeper, out.ZookeeperSource)
	}
	return nil
}

func convertZookeeperSource(in *Zookeeper, out *bootstrap.ZookeeperSourceArgs) {
	var sourcedEpSelectors []*bootstrap.EndpointSelector
	for _, i := range in.AvailableInterfaces {
		var interfaceSelector bootstrap.EndpointSelector = bootstrap.EndpointSelector{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
		}
		if i.Interface != "" {
			interfaceSelector.MatchLabels["interface"] = i.Interface
		}
		if i.Group != "" {
			interfaceSelector.MatchLabels["group"] = i.Group
		}
		if i.Version != "" {
			interfaceSelector.MatchLabels["version"] = i.Version
		}
		if len(interfaceSelector.MatchLabels) > 0 {
			sourcedEpSelectors = append(sourcedEpSelectors, &interfaceSelector)
		}
	}
	excludeIPRanges := &bootstrap.IPRanges{}
	for _, ips := range in.GlobalAbnormalInstanceIPs {
		if len(ips) > 0 {
			excludeIPRanges.IPs = append(excludeIPRanges.IPs, ips...)
		}
	}
	if len(excludeIPRanges.IPs) > 0 {
		if len(sourcedEpSelectors) == 0 {
			sourcedEpSelectors = append(sourcedEpSelectors, &bootstrap.EndpointSelector{
				ExcludeIPRanges: excludeIPRanges,
			})
		} else {
			for _, selector := range sourcedEpSelectors {
				selector.ExcludeIPRanges = excludeIPRanges
			}
		}
	}

	var servicedEndpointSelector = map[string][]*bootstrap.EndpointSelector{}
	for igv, ips := range in.AbnormalInstanceIPs {
		if len(ips) > 0 {
			servicedEndpointSelector[igv] = []*bootstrap.EndpointSelector{
				{
					ExcludeIPRanges: &bootstrap.IPRanges{
						IPs: ips,
					},
				},
			}
		}
	}
	out.EndpointSelectors = sourcedEpSelectors
	out.ServicedEndpointSelectors = servicedEndpointSelector
}
