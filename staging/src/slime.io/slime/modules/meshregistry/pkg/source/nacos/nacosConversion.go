package nacos

import (
	"net"
	"sort"
	"strings"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
	networkingapi "istio.io/api/networking/v1alpha3"

	"slime.io/slime/modules/meshregistry/pkg/util"
)

func ConvertServiceEntryMapForNacos(service string, instances []model.Instance, svcNameWithNs bool, gatewayModel bool, svcPort uint32, nsHost bool, k8sDomainSuffix bool, patchLabel bool) (map[string]*networkingapi.ServiceEntry, error) {
	seMap := make(map[string]*networkingapi.ServiceEntry, 0)
	if len(instances) == 0 {
		return seMap, nil
	}
	if gatewayModel {
		seMap[service] = convertServiceEntryForNacos(service, instances, nsHost, patchLabel)
	} else {
		for k, v := range convertServiceEntryWithNsForNacos(service, instances, svcPort, nsHost, k8sDomainSuffix, svcNameWithNs, patchLabel) {
			seMap[k] = v
		}
	}
	return seMap, nil
}

// -------- for gateway mode --------
func convertServiceEntryForNacos(service string, instances []model.Instance, nsHost bool, patchLabel bool) *networkingapi.ServiceEntry {
	endpoints, ports, _ := convertEndpointsForNacos(service, instances, patchLabel)
	nsSuffix := ""
	if nsHost {
		nsSuffix = ".nacos"
	}
	return &networkingapi.ServiceEntry{
		Hosts:      []string{strings.ReplaceAll(strings.ToLower(service), "_", "-") + nsSuffix},
		Resolution: networkingapi.ServiceEntry_DNS,
		Endpoints:  endpoints,
		Ports:      ports,
	}
}

func convertEndpointsForNacos(service string, instances []model.Instance, patchLabel bool) ([]*networkingapi.WorkloadEntry, []*networkingapi.ServicePort, []string) {
	endpoints := make([]*networkingapi.WorkloadEntry, 0)
	ports := make([]*networkingapi.ServicePort, 0)
	address := make([]string, 0)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})
	port := &networkingapi.ServicePort{
		Protocol: "HTTP",
		Number:   80,
		Name:     "http",
	}
	ports = append(ports, port)
	for _, ins := range instances {
		if !ins.Healthy {
			continue
		}

		instancePorts := make(map[string]uint32, 1)
		for _, v := range ports {
			instancePorts[v.Name] = uint32(ins.Port)
		}

		address = append(address, ins.Ip)

		convertInstanceId(ins.Metadata)

		util.FilterLabels(ins.Metadata, patchLabel, ins.Ip, "nacos :"+ins.InstanceId)

		ep := &networkingapi.WorkloadEntry{
			Address: ins.Ip,
			Ports:   instancePorts,
			Labels:  ins.Metadata,
		}
		util.FillWorkloadEntryLocality(ep)

		endpoints = append(endpoints, ep)
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Number < ports[j].Number
	})
	return endpoints, ports, address
}

// -------- for sidecar mode --------
func convertServiceEntryWithNsForNacos(service string, instances []model.Instance, svcPort uint32, nsHost bool,
	k8sDomainSuffix bool, svcNameWithNs bool, patchLabel bool,
) map[string]*networkingapi.ServiceEntry {
	endpointMap, portMap, useDNSMap := convertEndpointsWithNsForNacos(service, instances, svcPort, svcNameWithNs, patchLabel)
	if len(endpointMap) > 0 {
		ses := make(map[string]*networkingapi.ServiceEntry, len(endpointMap))
		for ns, endpoints := range endpointMap {
			seName := service
			nsSuffix := ""
			if !svcNameWithNs {
				if nsHost {
					nsSuffix = "." + ns
				}
				if k8sDomainSuffix {
					nsSuffix = nsSuffix + ".svc.cluster.local"
				}
			} else {
				nsSuffix = ".svc.cluster.local"
			}
			resolution := networkingapi.ServiceEntry_STATIC
			if useDNSMap[ns] {
				resolution = networkingapi.ServiceEntry_DNS
			}
			if !svcNameWithNs && ns != "" {
				seName = seName + "." + ns
			}
			ses[seName] = &networkingapi.ServiceEntry{
				Hosts:      []string{service + nsSuffix},
				Resolution: resolution,
				Endpoints:  endpoints,
				Ports:      portMap[ns],
			}
		}
		return ses
	}
	return nil
}

func convertEndpointsWithNsForNacos(service string, instances []model.Instance, svcPort uint32, svcNameWithNs bool, patchLabel bool) (map[string][]*networkingapi.WorkloadEntry, map[string][]*networkingapi.ServicePort, map[string]bool) {
	endpointsMap := make(map[string][]*networkingapi.WorkloadEntry, 0)
	portsMap := make(map[string][]*networkingapi.ServicePort, 0)
	useDNSMap := make(map[string]bool, 0)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})
	ns := ""
	if svcNameWithNs {
		item := strings.Split(service, ".")
		ns = item[len(item)-1]
	}
	for _, ins := range instances {
		if !ins.Healthy {
			continue
		}

		metadata := ins.Metadata
		convertInstanceId(metadata)
		util.FilterLabels(metadata, patchLabel, ins.Ip, "nacos :"+ins.InstanceId)

		if ns == "" {
			nsInLabel, exist := metadata["k8sNs"]
			if !exist {
				nsInLabel = "nacos"
			}
			ns = nsInLabel
		}

		endpoints, exist := endpointsMap[ns]
		if !exist {
			endpoints = make([]*networkingapi.WorkloadEntry, 0)
		}

		ports, exist := portsMap[ns]
		if !exist {
			portNum := svcPort
			if svcPort == 0 {
				portNum = uint32(ins.Port)
			}
			ports = make([]*networkingapi.ServicePort, 0)
			port := &networkingapi.ServicePort{
				Protocol: "HTTP",
				Number:   portNum,
				Name:     "http",
			}
			ports = append(ports, port)
			portsMap[ns] = ports
		}

		instancePorts := make(map[string]uint32, 1)
		instancePorts["http"] = uint32(ins.Port)

		useDns, e := useDNSMap[ns]
		if !e {
			useDns = false
			useDNSMap[ns] = false
		}
		if !useDns {
			ipAdd := net.ParseIP(ins.Ip)
			if ipAdd == nil {
				useDNSMap[ns] = true
			}
		}

		ep := &networkingapi.WorkloadEntry{
			Address: ins.Ip,
			Ports:   instancePorts,
			Labels:  ins.Metadata,
		}
		util.FillWorkloadEntryLocality(ep)
		endpoints = append(endpoints, ep)

		endpointsMap[ns] = endpoints
	}
	return endpointsMap, portsMap, useDNSMap
}

func convertInstanceIdForNacos(labels map[string]string) {
	v, ok := labels["instanceId"]
	if ok {
		labels["instanceId"] = strings.ReplaceAll(v, ":", "_")
	}
}
