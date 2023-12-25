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

func ConvertServiceEntryMap(
	instances []*instanceResp, defaultSvcNs string, gatewayModel bool, svcPort uint32, nsHost, k8sDomainSuffix,
	instancePortAsSvcPort, patchLabel, nsfNacos bool, filter func(*instance) bool, hostAliases map[string][]string,
) (map[string]*networking.ServiceEntry, error) {
	seMap := make(map[string]*networking.ServiceEntry, 0)
	if len(instances) == 0 {
		return seMap, nil
	}
	for _, ins := range instances {
		if gatewayModel {
			if nsfNacos {
				// 支持租户隔离
				for _, projectCode := range getProjectCode(ins) {
					seMap[ins.Dom+"-"+projectCode] = convertServiceEntryWithProjectCode(ins, nsHost, patchLabel, filter, hostAliases, projectCode)
				}
			} else {
				seMap[ins.Dom] = convertServiceEntryWithProjectCode(ins, nsHost, patchLabel, filter, hostAliases, "")
			}
		} else {
			for k, v := range convertServiceEntryWithNs(
				ins, defaultSvcNs, svcPort, nsHost, k8sDomainSuffix, instancePortAsSvcPort, patchLabel, filter,
				hostAliases) {
				seMap[k] = v
			}
		}
	}

	for _, se := range seMap {
		source.ApplyServicePortToEndpoints(se)
		source.RectifyServiceEntry(se)
	}
	return seMap, nil
}

// -------- for gateway mode --------
func convertServiceEntryWithProjectCode(instanceResp *instanceResp, nsHost bool, patchLabel bool, filter func(*instance) bool, hostAliases map[string][]string, projectCode string) *networking.ServiceEntry {
	endpoints, ports, _, hasNonIPEpAddr := convertEndpoints(instanceResp.Hosts, patchLabel, filter, projectCode)
	nsSuffix := ""
	if nsHost {
		nsSuffix = ".nacos"
	}
	host := strings.ReplaceAll(instanceResp.Dom, "_", "-")
	if projectCode != "" {
		host += ".nsf." + projectCode
	}
	host = strings.ToLower(host) + nsSuffix
	hosts := []string{host}
	if hostAliases != nil {
		hosts = append(hosts, hostAliases[host]...)
	}
	ret := &networking.ServiceEntry{
		Hosts:      hosts,
		Resolution: networking.ServiceEntry_STATIC,
		Endpoints:  endpoints,
		Ports:      ports,
	}
	if hasNonIPEpAddr {
		ret.Resolution = networking.ServiceEntry_DNS
	}
	return ret
}

func convertEndpoints(instances []*instance, patchLabel bool, filter func(*instance) bool, projectCode string) ([]*networking.WorkloadEntry, []*networking.ServicePort, []string, bool) {
	var (
		endpoints      = make([]*networking.WorkloadEntry, 0)
		ports          = make([]*networking.ServicePort, 0)
		address        = make([]string, 0)
		hasNonIPEpAddr bool
	)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})

	port := &networking.ServicePort{
		Protocol: "HTTP",
		Number:   80,
		Name:     "http",
	}
	ports = append(ports, port)

	for _, ins := range instances {
		if filter != nil && !filter(ins) {
			continue
		}

		if !ins.Healthy {
			continue
		}
		if ins.Port > math.MaxUint16 {
			log.Errorf("instance port illegal %v", ins)
			continue
		}

		if projectCode != "" && projectCode != ins.Metadata["projectCode"] {
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

func convertServiceEntryWithNs(
	instanceResp *instanceResp, defaultNs string, svcPort uint32, nsHost bool, k8sDomainSuffix bool,
	instancePortAsSvcPort, patchLabel bool, filter func(*instance) bool, hostAliases map[string][]string,
) map[string]*networking.ServiceEntry {
	endpointMap, nsSvcPorts, useDNSMap := convertEndpointsWithNs(
		instanceResp.Hosts, defaultNs, svcPort, nsHost, instancePortAsSvcPort, patchLabel, filter)
	if len(endpointMap) == 0 {
		return nil
	}

	if svcPort != 0 && instancePortAsSvcPort { // add extra svc port
		for _, svcPorts := range nsSvcPorts {
			if _, ok := svcPorts[svcPort]; !ok {
				svcPorts[svcPort] = &networking.ServicePort{
					Number:   svcPort,
					Protocol: source.ProtocolHTTP,
					Name:     source.PortName(source.ProtocolHTTP, svcPort),
				}
			}
		}
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
		hosts := []string{host}
		if hostAliases != nil {
			hosts = append(hosts, hostAliases[host]...)
		}

		portMap := nsSvcPorts[ns]
		ports := make([]*networking.ServicePort, 0, len(portMap))
		for _, p := range portMap {
			ports = append(ports, p)
		}
		sort.Slice(ports, func(i, j int) bool {
			return ports[i].Number < ports[j].Number
		})

		ses[seName] = &networking.ServiceEntry{
			Hosts:      hosts,
			Resolution: resolution,
			Endpoints:  endpoints,
			Ports:      ports,
		}
	}
	return ses
}

func convertEndpointsWithNs(
	instances []*instance, defaultNs string, svcPort uint32, nsHost, instancePortAsSvcPort, patchLabel bool,
	filter func(*instance) bool,
) (map[string][]*networking.WorkloadEntry, map[string]map[uint32]*networking.ServicePort, map[string]bool) {
	endpointsMap := make(map[string][]*networking.WorkloadEntry, 0)
	portsMap := make(map[string]map[uint32]*networking.ServicePort, 0)
	useDNSMap := make(map[string]bool, 0)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})

	for _, ins := range instances {
		if filter != nil && !filter(ins) {
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

		var svcPortName string
		ports, exist := portsMap[ns]
		if !exist {
			ports = map[uint32]*networking.ServicePort{}
			portsMap[ns] = ports
		}

		svcPortInUse := svcPort
		if instancePortAsSvcPort {
			svcPortInUse = uint32(ins.Port)
		}
		if v, ok := ports[svcPortInUse]; !ok {
			svcPortName = source.PortName(source.ProtocolHTTP, svcPortInUse)
			ports[svcPortInUse] = &networking.ServicePort{
				Protocol: source.ProtocolHTTP,
				Number:   svcPortInUse,
				Name:     svcPortName,
			}
		} else {
			svcPortName = v.Name
		}

		instancePorts := make(map[string]uint32, 1)
		instancePorts["http"] = uint32(ins.Port)

		ep := &networking.WorkloadEntry{
			Address: addr,
			Ports:   map[string]uint32{svcPortName: uint32(ins.Port)},
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

func getProjectCode(ins *instanceResp) []string {
	projectCode := make([]string, 0)
	projectCodeMap := make(map[string]string)
	for _, instance := range ins.Hosts {
		for k, v := range instance.Metadata {
			if k == "projectCode" {
				projectCodeMap[v] = ""
			}
		}
	}

	for k := range projectCodeMap {
		projectCode = append(projectCode, k)
	}
	return projectCode
}
