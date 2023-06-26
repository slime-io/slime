package generic

import (
	"math"
	"net"
	"sort"
	"strings"

	networking "istio.io/api/networking/v1alpha3"

	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

func convertServiceEntryMap[I Instance[I], APP Application[I, APP]](
	instances []APP, registry string, defaultSvcNs string, svcPort uint32,
	gatewayModel, nsfRegistry, nsHost, k8sDomainSuffix, patchLabel, instancePortAsSvcPort bool,
	filter func(I) bool, hostAliases map[string][]string,
) (map[string]*networking.ServiceEntry, error) {

	seMap := make(map[string]*networking.ServiceEntry, 0)
	if len(instances) == 0 {
		return seMap, nil
	}
	for _, ins := range instances {
		if gatewayModel {
			if nsfRegistry {
				for _, projectCode := range ins.GetProjectCodes() {
					seMap[ins.GetDomain()] = convertServiceEntryWithProjectCode(ins, registry, nsHost, "", patchLabel, projectCode, filter, hostAliases)
				}
			} else {
				seMap[ins.GetDomain()] = convertServiceEntry(ins, registry, nsHost, "", patchLabel, filter, hostAliases)
			}
		} else {
			for k, v := range convertServiceEntryWithNs(ins, registry, defaultSvcNs, svcPort,
				nsHost, k8sDomainSuffix, instancePortAsSvcPort, patchLabel,
				filter, hostAliases) {
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

// gateway
func convertServiceEntry[I Instance[I], APP Application[I, APP]](app APP, registry string, nsHost bool, defaultSuffix string, patchLabel bool,
	filter func(I) bool, hostAliases map[string][]string) *networking.ServiceEntry {
	endpoints, ports, _, hasNonIPEpAddr := convertEndpoints(app.GetInstances(), registry, patchLabel, "", filter)
	nsSuffix := ""
	if nsHost {
		nsSuffix = defaultSuffix
	}
	host := strings.ReplaceAll(strings.ToLower(app.GetDomain()), "_", "-") + nsSuffix
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

// gateway with projectCode
func convertServiceEntryWithProjectCode[I Instance[I], APP Application[I, APP]](app APP, registry string, nsHost bool, defaultSuffix string, patchLabel bool, projectCode string,
	filter func(I) bool, hostAliases map[string][]string) *networking.ServiceEntry {
	endpoints, ports, _, hasNonIPEpAddr := convertEndpoints(app.GetInstances(), registry, patchLabel, projectCode, filter)

	nsSuffix := ""
	if nsHost {
		nsSuffix = defaultSuffix
	}
	host := strings.ToLower(strings.ReplaceAll(app.GetDomain(), "_", "-")+".nsf."+projectCode) + nsSuffix
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
		// currently we do not consider the UDS case
		ret.Resolution = networking.ServiceEntry_DNS
	}

	return ret
}

func convertEndpoints[I Instance[I]](insts []I, registry string, patchLabel bool, projectCode string, filter func(I) bool) ([]*networking.WorkloadEntry, []*networking.Port, []string, bool) {
	var (
		endpoints      = make([]*networking.WorkloadEntry, 0)
		ports          = make([]*networking.Port, 0)
		address        = make([]string, 0)
		hasNonIPEpAddr bool
	)
	sort.Slice(insts, func(i, j int) bool {
		return insts[i].Less(insts[j])
	})
	port := &networking.Port{
		Protocol: "HTTP",
		Number:   80,
		Name:     "http",
	}
	ports = append(ports, port)
	for _, inst := range insts {
		if filter != nil && !filter(inst) {
			continue
		}

		if !inst.IsHealthy() {
			continue
		}

		if inst.GetPort() > math.MaxUint16 {
			log.Errorf("instance port illegal %v", inst)
			continue
		}

		meta := inst.GetMetadata()
		if projectCode != "" && meta[ProjectCode] != projectCode {
			continue
		}
		convertInstanceId(meta)

		instancePorts := make(map[string]uint32, 1)
		for _, v := range ports {
			instancePorts[v.Name] = uint32(inst.GetPort())
		}

		var ipAddr = inst.GetAddress()
		if net.ParseIP(ipAddr) == nil {
			ipAddr = ""
			hasNonIPEpAddr = true
		}
		util.FilterLabels(meta, patchLabel, ipAddr, registry+" :"+inst.GetInstanceID())
		address = append(address, inst.GetAddress())
		ep := &networking.WorkloadEntry{
			Address: inst.GetAddress(),
			Ports:   instancePorts,
			Labels:  meta,
		}
		util.FillWorkloadEntryLocality(ep)
		endpoints = append(endpoints, ep)
	}

	return endpoints, ports, address, hasNonIPEpAddr
}

// sidecar
func convertServiceEntryWithNs[I Instance[I], APP Application[I, APP]](app APP, registry string, defaultNs string, svcPort uint32,
	nsHost, k8sDomainSuffix, instancePortAsSvcPort, patchLabel bool,
	filter func(I) bool, hostAliases map[string][]string) map[string]*networking.ServiceEntry {
	endpointMap, nsSvcPorts, useDNSMap := convertEndpointsWithNs(
		app.GetInstances(), registry, defaultNs, svcPort, nsHost, instancePortAsSvcPort, patchLabel, filter)
	if len(endpointMap) == 0 {
		return nil
	}

	if svcPort != 0 && instancePortAsSvcPort { // add extra svc port
		for _, svcPorts := range nsSvcPorts {
			if _, ok := svcPorts[svcPort]; !ok {
				svcPorts[svcPort] = &networking.Port{
					Number:   svcPort,
					Protocol: source.ProtocolHTTP,
					Name:     source.PortName(source.ProtocolHTTP, svcPort),
				}
			}
		}
	}

	// todo: why transform to lowercase?
	svcShortName := strings.ToLower(app.GetDomain())
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
		ports := make([]*networking.Port, 0, len(portMap))
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

func convertEndpointsWithNs[I Instance[I]](
	insts []I, registry string,
	defaultNs string, svcPort uint32, nsHost, instancePortAsSvcPort,
	patchLabel bool,
	filter func(I) bool,
) (map[string][]*networking.WorkloadEntry, map[string]map[uint32]*networking.Port, map[string]bool) {
	endpointsMap := make(map[string][]*networking.WorkloadEntry, 0)
	portsMap := make(map[string]map[uint32]*networking.Port, 0)
	useDNSMap := make(map[string]bool, 0)
	sort.Slice(insts, func(i, j int) bool {
		return insts[i].Less(insts[j])
	})
	for _, inst := range insts {
		if filter != nil && !filter(inst) {
			continue
		}
		if !inst.IsHealthy() {
			continue
		}

		meta := inst.GetMetadata()
		convertInstanceId(meta)

		var ns string
		if nsHost {
			if v, ok := meta["k8sNs"]; ok {
				ns = v
			} else {
				ns = defaultNs
			}
		}

		var ipAddr = inst.GetAddress()
		// only invalid ip, consider as domain and need to use dns
		if net.ParseIP(ipAddr) == nil {
			ipAddr = ""
			if !useDNSMap[ns] {
				useDNSMap[ns] = true
			}
		}
		util.FilterLabels(meta, patchLabel, ipAddr, registry+" :"+inst.GetInstanceID())

		var svcPortName string
		ports, exist := portsMap[ns]
		if !exist {
			ports = map[uint32]*networking.Port{}
			portsMap[ns] = ports
		}
		svcPortInUse := svcPort
		if instancePortAsSvcPort {
			svcPortInUse = uint32(inst.GetPort())
		}
		if v, ok := ports[svcPortInUse]; !ok {
			svcPortName = source.PortName(source.ProtocolHTTP, svcPortInUse)
			ports[svcPortInUse] = &networking.Port{
				Protocol: source.ProtocolHTTP,
				Number:   svcPortInUse,
				Name:     svcPortName,
			}
		} else {
			svcPortName = v.Name
		}

		ep := &networking.WorkloadEntry{
			Address: inst.GetAddress(),
			Ports:   map[string]uint32{svcPortName: uint32(inst.GetPort())},
			Labels:  meta,
		}
		util.FillWorkloadEntryLocality(ep)
		endpointsMap[ns] = append(endpointsMap[ns], ep)
	}
	return endpointsMap, portsMap, useDNSMap
}

func convertInstanceId(meta map[string]string) {
	v, ok := meta["instanceId"]
	if ok {
		meta["instanceId"] = strings.ReplaceAll(v, ":", "_")
	}
}
