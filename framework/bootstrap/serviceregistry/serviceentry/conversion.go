package serviceentry

import (
	"strings"

	networking "istio.io/api/networking/v1alpha3"

	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
)

// ConvertSvcsAndEps transforms a ServiceEntry config to a list of IstioService and IstioEndpoint.
func ConvertSvcsAndEps(cfg resource.Config, seLabelSelectorKeys string) ([]*model.Service, []*model.IstioEndpoint) {
	serviceEntry := cfg.Spec.(*networking.ServiceEntry)

	outEps := convertIstioEndpoints(serviceEntry, cfg.Name, cfg.Namespace)
	outSvcs := make([]*model.Service, 0)

	svcPorts := make(model.PortList, 0, len(serviceEntry.Ports))
	for _, port := range serviceEntry.Ports {
		svcPorts = append(svcPorts, convertPort(port))
	}

	var labelSelectors map[string]string
	if serviceEntry.WorkloadSelector != nil {
		// labelSelectors from WorkloadSelector
		labelSelectors = serviceEntry.WorkloadSelector.Labels
	} else if seLabelSelectorKeys != "" {
		// labelSelectors from service entry labels
		keys := strings.Split(seLabelSelectorKeys, ",")
		labels := make(map[string]string, len(keys))
		for _, key := range keys {
			labels[key] = cfg.Labels[key]
		}
		labelSelectors = labels
	}
	// TODO - if serviceEntry does not have these labels, maybe we should get labels from spec.endpoints,
	// TODO - which kind is workloadEntry, has metadata labels

	for _, hostname := range serviceEntry.Hosts {
		svc := &model.Service{
			Hostname:  model.Name(hostname),
			Addresses: []string{model.UnspecifiedIP},
			Ports:     svcPorts,
			Attributes: model.ServiceAttributes{
				ServiceRegistry: model.External,
				Name:            cfg.Name,
				Namespace:       cfg.Namespace,
				Labels:          cfg.Labels,
				Annotations:     cfg.Annotations,
				LabelSelectors:  labelSelectors,
			},
		}
		if len(serviceEntry.Addresses) > 0 {
			svc.Addresses = serviceEntry.Addresses
		}

		svc.Endpoints = outEps
		outSvcs = append(outSvcs, svc)
	}

	return outSvcs, outEps
}

// convertIstioEndpoints transforms a ServiceEntry config to a list of IstioEndpoint.
func convertIstioEndpoints(serviceEntry *networking.ServiceEntry, svcName, svcNamespace string) []*model.IstioEndpoint {
	out := make([]*model.IstioEndpoint, 0)

	var hosts []model.Name
	for _, h := range serviceEntry.Hosts {
		hosts = append(hosts, model.Name(h))
	}

	if serviceEntry.Endpoints != nil {
		wles := serviceEntry.Endpoints
		for _, wle := range wles {
			for _, port := range serviceEntry.Ports {
				ep := convertIstioEndpoint(svcName, svcNamespace, port, wle, hosts)
				out = append(out, ep)
			}
		}
	} else if serviceEntry.WorkloadSelector != nil {
		// TODO ServiceEntry.Spec.WorkloadSelector -> match WorkloadEntry and Pod -> IstioEndpoints
	}

	return out
}

func convertIstioEndpoint(svcName, svcNamespace string, servicePort *networking.ServicePort,
	endpoint *networking.WorkloadEntry, hosts []model.Name,
) *model.IstioEndpoint {
	var instancePort uint32
	addr := endpoint.GetAddress()
	if strings.HasPrefix(addr, model.UnixAddressPrefix) {
		instancePort = 0
		addr = strings.TrimPrefix(addr, model.UnixAddressPrefix)
	} else if len(endpoint.Ports) > 0 { // endpoint port map takes precedence
		instancePort = endpoint.Ports[servicePort.Name]
		if instancePort == 0 {
			instancePort = servicePort.Number
		}
	} else if servicePort.TargetPort > 0 {
		instancePort = servicePort.TargetPort
	} else {
		// final fallback is to the service port value
		instancePort = servicePort.Number
	}

	return &model.IstioEndpoint{
		Address:         addr,
		EndpointPort:    instancePort,
		Hostnames:       hosts,
		Labels:          endpoint.Labels,
		Namespace:       svcNamespace,
		ServiceName:     svcName,
		ServicePortName: servicePort.Name,
	}
}

func convertPort(port *networking.ServicePort) *model.Port {
	return &model.Port{
		Name:     port.Name,
		Port:     int(port.Number),
		Protocol: model.Parse(port.Protocol),
	}
}
