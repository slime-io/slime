package serviceentry

import (
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"

	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
)

// ConvertSvcsAndEps transforms a ServiceEntry config to a list of IstioService and IstioEndpoint.
func ConvertSvcsAndEps(cfg resource.Config, seLabelSelectorKeys string) ([]*model.Service, []*model.IstioEndpoint) {
	serviceEntry := cfg.Spec.(*networkingapi.ServiceEntry)

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
func convertIstioEndpoints(serviceEntry *networkingapi.ServiceEntry, svcName, svcNs string) []*model.IstioEndpoint {
	out := make([]*model.IstioEndpoint, 0)

	var hosts []model.Name
	for _, h := range serviceEntry.Hosts {
		hosts = append(hosts, model.Name(h))
	}

	if serviceEntry.Endpoints != nil {
		wles := serviceEntry.Endpoints
		for _, wle := range wles {
			for _, port := range serviceEntry.Ports {
				ep := convertIstioEndpoint(svcName, svcNs, port, wle, hosts)
				out = append(out, ep)
			}
		}
	} else if serviceEntry.WorkloadSelector != nil { //nolint: staticcheck,revive
		// TODO ServiceEntry.Spec.WorkloadSelector -> match WorkloadEntry and Pod -> IstioEndpoints
	}

	return out
}

func convertIstioEndpoint(svcName, svcNamespace string, servicePort *networkingapi.ServicePort,
	endpoint *networkingapi.WorkloadEntry, hosts []model.Name,
) *model.IstioEndpoint {
	// default use servicePort.Number as endpoint port name
	instancePort := servicePort.Number
	addr := endpoint.GetAddress()

	// if targetPort is set, use it instead
	if servicePort.TargetPort > 0 {
		instancePort = servicePort.TargetPort
	}

	// endpoint port map takes precedence then targetPort
	if len(endpoint.Ports) > 0 {
		epPort := endpoint.Ports[servicePort.Name]
		if epPort != 0 {
			instancePort = servicePort.Number
		}
	}

	// if addr is unix socket, use 0 as endpoint port name
	if strings.HasPrefix(addr, model.UnixAddressPrefix) {
		instancePort = 0
		addr = strings.TrimPrefix(addr, model.UnixAddressPrefix)
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

func convertPort(port *networkingapi.ServicePort) *model.Port {
	return &model.Port{
		Name:     port.Name,
		Port:     int(port.Number),
		Protocol: model.Parse(port.Protocol),
	}
}
