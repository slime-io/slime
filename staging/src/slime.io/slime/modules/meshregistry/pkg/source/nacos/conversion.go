package nacos

import (
	"net"
	"sort"
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"

	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

const (
	ProjectCode = "projectCode"
)

type Error struct {
	msg string
}

func (e Error) Error() string {
	return e.msg
}

type convertOptions struct {
	patchLabel            bool
	enableProjectCode     bool
	nsHost                bool
	k8sDomainSuffix       bool
	instancePortAsSvcPort bool
	svcPort               uint32
	defaultSvcNs          string
	domSuffix             string

	// the protocol used for Port.Protocol
	protocol string
	// the protocol name used for Port.Name
	protocolName string

	filter      func(*instance) bool
	hostAliases map[string][]string
}

func ConvertServiceEntryMap(instances []*instanceResp, opts *convertOptions) (map[string]*networkingapi.ServiceEntry, error) {
	seMap := make(map[string]*networkingapi.ServiceEntry, 0)
	if len(instances) == 0 {
		return seMap, nil
	}
	for _, ins := range instances {
		correctedDom := strings.ReplaceAll(strings.ToLower(ins.Dom), "_", "-")
		if opts.domSuffix != "" {
			correctedDom = correctedDom + "." + opts.domSuffix
		}

		var projectCodes []string
		if opts.enableProjectCode {
			projectCodes = getProjectCodeArr(ins)
		} else {
			projectCodes = append(projectCodes, "")
		}

		for _, projectCode := range projectCodes {
			projectDom := correctedDom
			if projectCode != "" {
				projectDom = projectDom + "." + projectCode
			}

			projectIns := *ins
			projectIns.Dom = projectDom

			for k, v := range convertServiceEntry(&projectIns, projectCode, opts) {
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

func convertServiceEntry(instanceResp *instanceResp, projectCode string, opts *convertOptions) map[string]*networkingapi.ServiceEntry {
	nsEndpoints, nsSvcPorts, useDNSMap := convertEndpointsWithNs(instanceResp.Hosts, projectCode, opts)
	if len(nsEndpoints) == 0 {
		return nil
	}

	if opts.svcPort != 0 && opts.instancePortAsSvcPort { // add extra svc port
		for _, svcPorts := range nsSvcPorts {
			if _, ok := svcPorts[opts.svcPort]; !ok {
				svcPorts[opts.svcPort] = &networkingapi.ServicePort{
					Number:   opts.svcPort,
					Protocol: opts.protocol,
					Name:     source.PortName(opts.protocolName, opts.svcPort),
				}
			}
		}
	}

	var (
		ses          = make(map[string]*networkingapi.ServiceEntry, len(nsEndpoints))
		svcShortName = instanceResp.Dom
	)

	for ns, endpoints := range nsEndpoints {
		var (
			host   = svcShortName
			seName = svcShortName
		)
		if opts.nsHost && ns != "" {
			seName += "." + ns
			host += "." + ns
			if opts.k8sDomainSuffix {
				host += ".svc.cluster.local"
			}
		}

		resolution := networkingapi.ServiceEntry_STATIC
		if useDNSMap[ns] {
			resolution = networkingapi.ServiceEntry_DNS
		}

		existSe := ses[seName]
		if existSe != nil {
			// should never happen considering that we've done merge-ns in convert-ep stage
			log.Errorf("found dup se %s, prev %+v", seName, existSe)
		} else {
			hosts := []string{host}
			if opts.hostAliases != nil {
				hosts = append(hosts, opts.hostAliases[host]...)
			}

			portMap := nsSvcPorts[ns]
			ports := make([]*networkingapi.ServicePort, 0, len(portMap))
			for _, p := range portMap {
				ports = append(ports, p)
			}
			sort.Slice(ports, func(i, j int) bool {
				return ports[i].Number < ports[j].Number
			})

			ses[seName] = &networkingapi.ServiceEntry{
				Hosts:      hosts,
				Resolution: resolution,
				Endpoints:  endpoints,
				Ports:      ports,
			}
		}
	}

	return ses
}

func convertEndpointsWithNs(instances []*instance, projectCode string, opts *convertOptions,
) (map[string][]*networkingapi.WorkloadEntry, map[string]map[uint32]*networkingapi.ServicePort, map[string]bool) {
	endpointsMap := make(map[string][]*networkingapi.WorkloadEntry, 0)
	svcPortsMap := make(map[string]map[uint32]*networkingapi.ServicePort, 0)
	useDNSMap := make(map[string]bool, 0)
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceId < instances[j].InstanceId
	})

	for _, ins := range instances {
		if opts.filter != nil && !opts.filter(ins) {
			continue
		}
		if !ins.Healthy { // nacos-spec
			continue
		}
		// 与要求projectCode不同的服务实例跳过，实现服务实例项目隔离
		if projectCode != "" && ins.Metadata[ProjectCode] != projectCode {
			continue
		}

		metadata := ins.Metadata
		convertInstanceId(metadata) // nacos-spec
		util.FilterEndpointLabels(metadata, opts.patchLabel, ins.Ip, "nacos :"+ins.InstanceId)

		var ns string
		if opts.nsHost {
			if v, ok := metadata["k8sNs"]; ok {
				ns = v
			} else {
				ns = opts.defaultSvcNs // "nacos" in old impl
			}
		}

		var svcPortName string
		ports, exist := svcPortsMap[ns]
		if !exist {
			ports = map[uint32]*networkingapi.ServicePort{}
			svcPortsMap[ns] = ports
		}

		svcPortInUse := opts.svcPort
		if opts.instancePortAsSvcPort {
			svcPortInUse = uint32(ins.Port)
		}
		if v, ok := ports[svcPortInUse]; !ok {
			svcPortName = source.PortName(opts.protocolName, svcPortInUse)
			ports[svcPortInUse] = &networkingapi.ServicePort{
				Protocol: opts.protocol,
				Number:   svcPortInUse,
				Name:     svcPortName,
			}
		} else {
			svcPortName = v.Name
		}

		if useDns := useDNSMap[ns]; !useDns {
			ipAdd := net.ParseIP(ins.Ip)
			if ipAdd == nil { // invalid ip, consider as domain and need to use dns
				useDNSMap[ns] = true
			}
		}

		if useDns := useDNSMap[ns]; !useDns {
			ipAdd := net.ParseIP(ins.Ip)
			if ipAdd == nil { // invalid ip, consider as domain and need to use dns
				useDNSMap[ns] = true
			}
		}

		ep := &networkingapi.WorkloadEntry{
			Address: ins.Ip,
			Ports:   map[string]uint32{svcPortName: uint32(ins.Port)},
			Labels:  ins.Metadata,
		}

		util.FillWorkloadEntryLocality(ep)

		endpointsMap[ns] = append(endpointsMap[ns], ep)
	}

	return endpointsMap, svcPortsMap, useDNSMap
}

func convertInstanceId(labels map[string]string) {
	v, ok := labels["instanceId"]
	if ok {
		labels["instanceId"] = strings.ReplaceAll(v, ":", "_")
	}
}

func getProjectCodeArr(ins *instanceResp) []string {
	projectCodes := make([]string, 0)
	projectCodeMap := make(map[string]struct{})
	for _, instance := range ins.Hosts {
		if v, ok := instance.Metadata[ProjectCode]; ok {
			if _, ok := projectCodeMap[v]; !ok {
				projectCodes = append(projectCodes, v)
				projectCodeMap[v] = struct{}{}
			}
		}
	}

	return projectCodes
}
