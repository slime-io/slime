package eureka

import (
	"math"
	"net"
	"sort"
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"

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
	apps []*application, defaultSvcNs, appSuffix string, patchLabel bool, svcPort uint32,
	instancePortAsSvcPort, nsHost, k8sDomainSuffix, enableProjectCode bool,
) (map[string]*networkingapi.ServiceEntry, error) {
	seMap := make(map[string]*networkingapi.ServiceEntry, 0)
	if apps == nil || len(apps) == 0 {
		return seMap, nil
	}
	for _, app := range apps {
		correctedAppName := strings.ReplaceAll(strings.ToLower(app.Name), "_", "-")
		if appSuffix != "" {
			correctedAppName = correctedAppName + "." + appSuffix
		}

		var projectCodes []string
		if enableProjectCode {
			projectCodes = getProjectCodeArr(app)
		} else {
			projectCodes = append(projectCodes, "")
		}

		for _, projectCode := range projectCodes {
			projectAppName := correctedAppName
			if projectCode != "" {
				projectAppName = projectAppName + "." + projectCode
			}

			projectApp := *app
			projectApp.Name = projectAppName

			for k, v := range convertServiceEntry(
				&projectApp, defaultSvcNs, projectCode, svcPort, nsHost, k8sDomainSuffix, instancePortAsSvcPort, patchLabel) {
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

func convertServiceEntry(
	app *application, defaultNs, projectCode string, svcPort uint32,
	nsHost, k8sDomainSuffix, instancePortAsSvcPort, patchLabel bool,
) map[string]*networkingapi.ServiceEntry {
	nsEndpoints, nsSvcPorts, nsUseDnsMap := convertEndpointsWithNs(
		app.Instances, defaultNs, projectCode, svcPort, nsHost, instancePortAsSvcPort, patchLabel)
	if len(nsEndpoints) == 0 {
		return nil
	}

	if svcPort != 0 && instancePortAsSvcPort { // add extra svc port
		for _, svcPorts := range nsSvcPorts {
			if _, ok := svcPorts[svcPort]; !ok {
				svcPorts[svcPort] = &networkingapi.ServicePort{
					Number:   svcPort,
					Protocol: source.ProtocolHTTP,
					Name:     source.PortName(source.ProtocolHTTP, svcPort),
				}
			}
		}
	}

	var (
		ses          = make(map[string]*networkingapi.ServiceEntry, len(nsEndpoints))
		svcShortName = app.Name
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

		resolution := networkingapi.ServiceEntry_STATIC
		if nsUseDnsMap[ns] {
			resolution = networkingapi.ServiceEntry_DNS
		}

		existSe := ses[seName]
		if existSe != nil {
			// should never happen considering that we've done merge-ns in convert-ep stage
			log.Errorf("eureka found dup se %s, prev %+v", seName, existSe)
		} else {
			portMap := nsSvcPorts[ns]
			ports := make([]*networkingapi.ServicePort, 0, len(portMap))
			for _, p := range portMap {
				ports = append(ports, p)
			}
			sort.Slice(ports, func(i, j int) bool {
				return ports[i].Number < ports[j].Number
			})

			ses[seName] = &networkingapi.ServiceEntry{
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
	instances []*instance, defaultNs, projectCode string, svcPort uint32, withNs, instancePortAsSvcPort,
	patchLabel bool,
) (map[string][]*networkingapi.WorkloadEntry, map[string]map[uint32]*networkingapi.ServicePort, map[string]bool) {
	endpointsMap := make(map[string][]*networkingapi.WorkloadEntry, 0)
	svcPortsMap := make(map[string]map[uint32]*networkingapi.ServicePort, 0)
	useDNSMap := make(map[string]bool, 0)

	sort.Slice(instances, func(i, j int) bool {
		return instances[i].InstanceID < instances[j].InstanceID
	})

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

		metadata := ins.Metadata
		util.FilterLabels(metadata, patchLabel, ins.IPAddress, "eureka:"+ins.InstanceID)

		var ns string
		if withNs {
			if v, ok := metadata["k8sNs"]; ok {
				ns = v
			} else {
				ns = defaultNs // "eureka" in old impl
			}
		}

		var svcPortName string
		ports, exist := svcPortsMap[ns]
		if !exist {
			ports = map[uint32]*networkingapi.ServicePort{}
			svcPortsMap[ns] = ports
		}

		svcPortInUse := svcPort
		if instancePortAsSvcPort {
			svcPortInUse = uint32(ins.Port.Port)
		}
		if v, ok := ports[svcPortInUse]; !ok {
			svcPortName = source.PortName(source.ProtocolHTTP, svcPortInUse)
			ports[svcPortInUse] = &networkingapi.ServicePort{
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

		ep := &networkingapi.WorkloadEntry{
			Address: ins.IPAddress,
			Ports:   map[string]uint32{svcPortName: uint32(ins.Port.Port)},
			Labels:  ins.Metadata,
		}

		util.FillWorkloadEntryLocality(ep)

		endpointsMap[ns] = append(endpointsMap[ns], ep)
	}

	return endpointsMap, svcPortsMap, useDNSMap
}

// 获取每一个应用的projectCode
func getProjectCodeArr(app *application) []string {
	projectCodes := make([]string, 0)
	projectCodeMap := make(map[string]struct{})
	// 获取服务中所有实例的projectCode标签，并去重
	for _, instance := range app.Instances {
		if v, ok := instance.Metadata[ProjectCode]; ok {
			if _, ok = projectCodeMap[v]; !ok {
				projectCodes = append(projectCodes, v)
				projectCodeMap[v] = struct{}{}
			}
		}
	}

	return projectCodes
}
