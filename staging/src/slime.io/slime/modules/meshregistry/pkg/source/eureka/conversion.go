package eureka

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

const (
	ProjectCode = "projectCode"
)

func (e Error) Error() string {
	return e.msg
}

func ConvertServiceEntryMap(
	apps []*application, defaultSvcNs string, gatewayModel, patchLabel bool, svcPort uint32,
	instancePortAsSvcPort, nsHost, k8sDomainSuffix, nsfEureka bool,
) (map[string]*networking.ServiceEntry, error) {
	seMap := make(map[string]*networking.ServiceEntry, 0)
	if apps == nil || len(apps) == 0 {
		return seMap, nil
	}
	for _, app := range apps {
		if gatewayModel {
			if nsfEureka {
				// 基于projectCode支持eureka服务租户隔离
				for _, projectCode := range getProjectCodeArr(app) {
					seMap[app.Name+"-"+projectCode] = convertServiceEntryWithProjectCode(app, nsHost, patchLabel, projectCode)
				}
			} else {
				seMap[app.Name] = convertServiceEntry(app, nsHost, patchLabel)
			}
		} else {
			for k, v := range convertServiceEntryWithNs(
				app, defaultSvcNs, svcPort, nsHost, k8sDomainSuffix, instancePortAsSvcPort, patchLabel) {
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
func convertServiceEntry(app *application, nsHost bool, patchLabel bool) *networking.ServiceEntry {
	endpoints, ports, _, hasNonIPEpAddr := convertEndpoints(app.Instances, patchLabel, "")
	nsSuffix := ""
	if nsHost {
		nsSuffix = ".eureka"
	}

	ret := &networking.ServiceEntry{
		Hosts:      []string{strings.ReplaceAll(strings.ToLower(app.Name), "_", "-") + nsSuffix},
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

// 根据projectCode做eureka服务的项目隔离
func convertServiceEntryWithProjectCode(app *application, nsHost bool, patchLabel bool, projectCode string) *networking.ServiceEntry {
	endpoints, ports, _, hasNonIPEpAddr := convertEndpoints(app.Instances, patchLabel, projectCode)
	nsSuffix := ""
	if nsHost {
		nsSuffix = ".eureka"
	}

	ret := &networking.ServiceEntry{
		Hosts:      []string{strings.ToLower(strings.ReplaceAll(strings.ToLower(app.Name), "_", "-")+".nsf."+projectCode) + nsSuffix},
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

func convertEndpoints(instances []*instance, patchLabel bool, projectCode string) ([]*networking.WorkloadEntry, []*networking.ServicePort, []string, bool) {
	var (
		endpoints      = make([]*networking.WorkloadEntry, 0)
		ports          = make([]*networking.ServicePort, 0)
		address        = make([]string, 0)
		hasNonIPEpAddr bool
	)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceID < instances[j].InstanceID
	})

	port := &networking.ServicePort{
		Protocol: "HTTP",
		Number:   80,
		Name:     "http",
	}
	ports = append(ports, port)

	for _, ins := range instances {
		if !strings.EqualFold(ins.Status, "UP") {
			continue
		}
		if ins.Port.Port > math.MaxUint16 {
			log.Errorf("instance port illegal %v", ins)
			continue
		}
		// 与要求projectCode不同的服务实例跳过，实现eureka服务实例项目隔离
		if projectCode != "" && ins.Metadata[ProjectCode] != projectCode {
			continue
		}

		instancePorts := make(map[string]uint32, 1)
		for _, v := range ports {
			instancePorts[v.Name] = uint32(ins.Port.Port)
		}

		var (
			addr   = ins.IPAddress
			ipAddr = addr
		)
		if net.ParseIP(addr) == nil {
			ipAddr = ""
			hasNonIPEpAddr = true
		}
		address = append(address, ins.IPAddress)

		util.FilterLabels(ins.Metadata, patchLabel, ipAddr, "eureka:"+ins.InstanceID)

		ep := &networking.WorkloadEntry{
			Address: ins.IPAddress,
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
func convertServiceEntryWithNs(
	app *application, defaultNs string, svcPort uint32,
	nsHost, k8sDomainSuffix, instancePortAsSvcPort, patchLabel bool,
) map[string]*networking.ServiceEntry {
	nsEndpoints, nsSvcPorts, nsUseDnsMap := convertEndpointsWithNs(
		app.Instances, defaultNs, svcPort, nsHost, instancePortAsSvcPort, patchLabel)
	if len(nsEndpoints) == 0 {
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

	var (
		ses          = make(map[string]*networking.ServiceEntry, len(nsEndpoints))
		svcShortName = strings.ToLower(app.Name)
	)

	for ns, endpoints := range nsEndpoints {
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
		if nsUseDnsMap[ns] {
			resolution = networking.ServiceEntry_DNS
		}

		existSe := ses[seName]
		if existSe != nil {
			// should never happen considering that we've done merge-ns in convert-ep stage
			log.Errorf("eureka found dup se %s, prev %+v", seName, existSe)
		} else {
			portMap := nsSvcPorts[ns]
			ports := make([]*networking.ServicePort, 0, len(portMap))
			for _, p := range portMap {
				ports = append(ports, p)
			}
			sort.Slice(ports, func(i, j int) bool {
				return ports[i].Number < ports[j].Number
			})

			ses[seName] = &networking.ServiceEntry{
				Hosts:      []string{host},
				Resolution: resolution,
				Endpoints:  endpoints,
				Ports:      ports,
			}
		}
	}

	return ses
}

func convertEndpointsWithNs(
	instances []*instance, defaultNs string, svcPort uint32, nsHost, instancePortAsSvcPort,
	patchLabel bool,
) (map[string][]*networking.WorkloadEntry, map[string]map[uint32]*networking.ServicePort, map[string]bool) {
	endpointsMap := make(map[string][]*networking.WorkloadEntry, 0)
	portsMap := make(map[string]map[uint32]*networking.ServicePort, 0)
	useDNSMap := make(map[string]bool, 0)

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceID < instances[j].InstanceID
	})

	for _, ins := range instances {
		if !strings.EqualFold(ins.Status, "UP") {
			continue
		}

		metadata := ins.Metadata
		util.FilterLabels(metadata, patchLabel, ins.IPAddress, "eureka:"+ins.InstanceID)

		var ns string
		if nsHost {
			if v, ok := metadata["k8sNs"]; ok {
				ns = v
			} else {
				ns = defaultNs // "eureka" in old impl
			}
		}

		var svcPortName string
		ports, exist := portsMap[ns]
		if !exist {
			ports = map[uint32]*networking.ServicePort{}
			portsMap[ns] = ports
		}

		svcPortInUse := svcPort
		if instancePortAsSvcPort {
			svcPortInUse = uint32(ins.Port.Port)
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

		if useDns := useDNSMap[ns]; !useDns {
			ipAdd := net.ParseIP(ins.IPAddress)
			if ipAdd == nil { // invalid ip, consider as domain and need to use dns
				useDNSMap[ns] = true
			}
		}

		ep := &networking.WorkloadEntry{
			Address: ins.IPAddress,
			Ports:   map[string]uint32{svcPortName: uint32(ins.Port.Port)},
			Labels:  ins.Metadata,
		}
		util.FillWorkloadEntryLocality(ep)

		endpointsMap[ns] = append(endpointsMap[ns], ep)
	}

	return endpointsMap, portsMap, useDNSMap
}

// 获取每一个应用的projectCode
func getProjectCodeArr(app *application) []string {
	projectCode := make([]string, 0)
	projectCodeMap := make(map[string]string)
	// 获取服务中所有实例的projectCode标签，并去重
	for _, instance := range app.Instances {
		for k, v := range instance.Metadata {
			if k == ProjectCode {
				projectCodeMap[v] = ""
			}
		}
	}

	for k := range projectCodeMap {
		projectCode = append(projectCode, k)
	}
	return projectCode
}
