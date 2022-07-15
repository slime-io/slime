package kube

import (
	v1 "k8s.io/api/core/v1"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
)

func buildIstioEndpoint(endpointAddress string, endpointPort int32, svcPortName, svcName, svcNamespace string,
	pod *v1.Pod, hosts []model.Name) *model.IstioEndpoint {
	if pod == nil {
		return nil
	}

	return &model.IstioEndpoint{
		Address:         endpointAddress,
		EndpointPort:    uint32(endpointPort),
		Hostnames:       hosts,
		Labels:          pod.Labels,
		ServicePortName: svcPortName,
		Namespace:       svcNamespace,
		ServiceName:     svcName,
	}
}
