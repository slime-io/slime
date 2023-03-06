package nacos

import (
	"math"
	"net"
	"sort"
	"strings"

	networking "istio.io/api/networking/v1alpha3"

	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

type Error struct {
	msg string
}

func (e Error) Error() string {
	return e.msg
}

func ConvertServiceEntryMap(instances []*instanceResp, defaultSvcNs string, gatewayModel bool, svcPort uint32, nsHost bool, k8sDomainSuffix bool, patchLabel bool, filters source.SelectHookStore) (map[string]*networking.ServiceEntry, error) {
	seMap := make(map[string]*networking.ServiceEntry, 0)
	if len(instances) == 0 {
		return seMap, nil
	}
	for _, ins := range instances {
		if gatewayModel {
			seMap[ins.Dom] = convertServiceEntry(ins, nsHost, patchLabel, filters)
		} else {
			for k, v := range convertServiceEntryWithNs(ins, defaultSvcNs, svcPort, nsHost, k8sDomainSuffix, patchLabel, filters) {
				seMap[k] = v
			}
		}
	}
	return seMap, nil
}

// -------- for gateway mode --------
func convertServiceEntry(instanceResp *instanceResp, nsHost bool, patchLabel bool, filters source.SelectHookStore) *networking.ServiceEntry {
	endpoints, ports, _, hasNonIPEpAddr := convertEndpoints(instanceResp.Hosts, patchLabel, filters)
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

func convertEndpoints(instances []*instance, patchLabel bool, filters source.SelectHookStore) ([]*networking.WorkloadEntry, []*networking.Port, []string, bool) {
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
		if filter := filters[allSeriveFilter]; filter != nil && !filter(ins.Metadata) {
			continue
		}
		if filter := filters[ins.ServiceName]; filter != nil && !filter(ins.Metadata) {
			continue
		}
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

func convertServiceEntryWithNs(instanceResp *instanceResp, defaultNs string, svcPort uint32, nsHost bool, k8sDomainSuffix bool, patchLabel bool, filters source.SelectHookStore) map[string]*networking.ServiceEntry {
	endpointMap, portMap, useDNSMap := convertEndpointsWithNs(instanceResp.Hosts, defaultNs, svcPort, nsHost, patchLabel, filters)
	if len(endpointMap) == 0 {
		return nil
	}

	// todo: why transform to lowercase?
	svcShortName := strings.ToLower(instanceResp.Dom)
	ses := make(map[string]*networking.ServiceEntry, len(endpointMap))
	for ns, endpoints := range endpointMap {
		var (
			host   = svcShortName
			seName = svcShortName
		)
		if nsHost && ns != "" {
			seName += "." + ns
			host += "." + ns
			if k8sDomainSuffix {
				host += ".svc.cluster.local"
			}
		}
		resolution := networking.ServiceEntry_STATIC
		if useDNSMap[ns] {
			resolution = networking.ServiceEntry_DNS
		}
		ses[seName] = &networking.ServiceEntry{
			Hosts:      []string{host},
			Resolution: resolution,
			Endpoints:  endpoints,
			Ports:      portMap[ns],
		}
	}
	return ses
}

func convertEndpointsWithNs(instances []*instance, defaultNs string, svcPort uint32, nsHost, patchLabel bool, filters source.SelectHookStore) (map[string][]*networking.WorkloadEntry, map[string][]*networking.Port, map[string]bool) {
	endpointsMap := make(map[string][]*networking.WorkloadEntry, 0)
	portsMap := make(map[string][]*networking.Port, 0)
	useDNSMap := make(map[string]bool, 0)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})

	for _, ins := range instances {
		if filter := filters[allSeriveFilter]; filter != nil && !filter(ins.Metadata) {
			continue
		}
		if filter := filters[ins.ServiceName]; filter != nil && !filter(ins.Metadata) {
			continue
		}
		if !ins.Healthy {
			continue
		}

		metadata := ins.Metadata
		convertInstanceId(metadata)

		var ns string
		if nsHost {
			if v, ok := metadata["k8sNs"]; ok {
				ns = v
			} else {
				ns = defaultNs // "nacos" in old impl
			}
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
