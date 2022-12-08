package nacos

import (
	"math"
	"net"
	"sort"
	"strings"

	networking "istio.io/api/networking/v1alpha3"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

type Error struct {
	msg string
}

func (e Error) Error() string {
	return e.msg
}

func ConvertServiceEntryMap(instances []*instanceResp, gatewayModel bool, svcPort uint32, nsHost bool, k8sDomainSuffix bool, patchLabel bool) (map[serviceEntryNameWapper]*networking.ServiceEntry, error) {
	seMap := make(map[serviceEntryNameWapper]*networking.ServiceEntry, 0)
	if len(instances) == 0 {
		return seMap, nil
	}
	for _, ins := range instances {
		if gatewayModel {
			sen := serviceEntryNameWapper{nacosService: ins.Dom}
			seMap[sen] = convertServiceEntry(ins, nsHost, patchLabel)
		} else {
			for k, v := range convertServiceEntryWithNs(ins, svcPort, nsHost, k8sDomainSuffix, patchLabel) {
				seMap[k] = v
			}
		}
	}
	return seMap, nil
}

// -------- for gateway mode --------
func convertServiceEntry(instanceResp *instanceResp, nsHost bool, patchLabel bool) *networking.ServiceEntry {
	endpoints, ports, _, hasNonIPEpAddr := convertEndpoints(instanceResp.Hosts, patchLabel)
	nsSuffix := ""
	if nsHost {
		nsSuffix = ".nacos"
	}
	ret := &networking.ServiceEntry{
		Hosts:      []string{strings.ReplaceAll(strings.ToLower(instanceResp.Dom), "_", "-") + nsSuffix},
		Resolution: networking.ServiceEntry_STATIC,
		Endpoints:  endpoints,
		Ports:      ports,
	}
	if hasNonIPEpAddr {
		ret.Resolution = networking.ServiceEntry_DNS
	}
	return ret
}

func convertEndpoints(instances []*instance, patchLabel bool) ([]*networking.WorkloadEntry, []*networking.Port, []string, bool) {
	var (
		endpoints      = make([]*networking.WorkloadEntry, 0)
		ports          = make([]*networking.Port, 0)
		address        = make([]string, 0)
		hasNonIPEpAddr bool
	)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})

	port := &networking.Port{
		Protocol: "HTTP",
		Number:   80,
		Name:     "http",
	}
	ports = append(ports, port)

	for _, ins := range instances {
		if !ins.Healthy {
			continue
		}
		if ins.Port > math.MaxUint16 {
			Scope.Errorf("instance port illegal %v", ins)
			continue
		}

		instancePorts := make(map[string]uint32, 1)
		for _, v := range ports {
			instancePorts[v.Name] = uint32(ins.Port)
		}

		var (
			addr   = ins.Ip
			ipAddr = addr
		)
		if net.ParseIP(addr) == nil {
			ipAddr = ""
			hasNonIPEpAddr = true
		}
		address = append(address, addr)

		convertInstanceId(ins.Metadata)

		util.FilterLabels(ins.Metadata, patchLabel, ipAddr, "nacos :"+ins.InstanceId)

		ep := &networking.WorkloadEntry{
			Address: addr,
			Ports:   instancePorts,
			Labels:  ins.Metadata,
		}
		util.FillWorkloadEntryLocality(ep)
		endpoints = append(endpoints, ep)
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Number < ports[j].Number
	})
	return endpoints, ports, address, hasNonIPEpAddr
}

// -------- for sidecar mode --------
func convertServiceEntryWithNs(instanceResp *instanceResp, svcPort uint32, nsHost bool, k8sDomainSuffix bool, patchLabel bool) map[serviceEntryNameWapper]*networking.ServiceEntry {
	endpointMap, portMap, useDNSMap := convertEndpointsWithNs(instanceResp.Hosts, svcPort, patchLabel)
	if len(endpointMap) == 0 {
		return nil
	}

	ses := make(map[serviceEntryNameWapper]*networking.ServiceEntry, len(endpointMap))
	for ns, endpoints := range endpointMap {
		nsSuffix := ""
		if nsHost {
			nsSuffix = "." + ns
		}
		if k8sDomainSuffix {
			nsSuffix = nsSuffix + ".svc.cluster.local"
		}
		resolution := networking.ServiceEntry_STATIC
		if useDNSMap[ns] {
			resolution = networking.ServiceEntry_DNS
		}
		snw := serviceEntryNameWapper{
			// todo: why transform to lowercase?
			nacosService: strings.ToLower(instanceResp.Dom),
			ns:           ns,
		}
		ses[snw] = &networking.ServiceEntry{
			Hosts:      []string{strings.ToLower(instanceResp.Dom) + nsSuffix},
			Resolution: resolution,
			Endpoints:  endpoints,
			Ports:      portMap[ns],
		}
	}
	return ses
}

func convertEndpointsWithNs(instances []*instance, svcPort uint32, patchLabel bool) (map[string][]*networking.WorkloadEntry, map[string][]*networking.Port, map[string]bool) {
	endpointsMap := make(map[string][]*networking.WorkloadEntry, 0)
	portsMap := make(map[string][]*networking.Port, 0)
	useDNSMap := make(map[string]bool, 0)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})
	for _, ins := range instances {
		if !ins.Healthy {
			continue
		}

		metadata := ins.Metadata
		convertInstanceId(metadata)

		ns, exist := metadata["k8sNs"]
		if !exist {
			ns = "nacos"
		}

		var (
			addr   = ins.Ip
			ipAddr = addr
		)
		useDns, e := useDNSMap[ns]
		if !e {
			useDns = false
			useDNSMap[ns] = false
		}
		if net.ParseIP(addr) == nil {
			ipAddr = ""
			if !useDns {
				useDns = true
				useDNSMap[ns] = true
			}
		}

		util.FilterLabels(metadata, patchLabel, ipAddr, "nacos :"+ins.InstanceId)

		endpoints, exist := endpointsMap[ns]
		if !exist {
			endpoints = make([]*networking.WorkloadEntry, 0)
		}

		ports, exist := portsMap[ns]
		if !exist {
			portNum := svcPort
			if svcPort == 0 {
				portNum = uint32(ins.Port)
			}
			port := &networking.Port{
				Protocol: "HTTP",
				Number:   portNum,
				Name:     "http",
			}
			ports = append(ports, port)
			portsMap[ns] = ports
		}

		instancePorts := make(map[string]uint32, 1)
		instancePorts["http"] = uint32(ins.Port)

		ep := &networking.WorkloadEntry{
			Address: addr,
			Ports:   instancePorts,
			Labels:  ins.Metadata,
		}
		util.FillWorkloadEntryLocality(ep)
		endpoints = append(endpoints, ep)

		endpointsMap[ns] = endpoints
	}
	return endpointsMap, portsMap, useDNSMap
}

func convertInstanceId(labels map[string]string) {
	v, ok := labels["instanceId"]
	if ok {
		labels["instanceId"] = strings.ReplaceAll(v, ":", "_")
	}
}
