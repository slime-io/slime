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

type convertOptions struct {
	patchLabel            bool
	enableProjectCode     bool
	nsHost                bool
	k8sDomainSuffix       bool
	instancePortAsSvcPort bool
	svcPort               uint32
	defaultSvcNs          string
	appSuffix             string

	// the protocol used for Port.Protocol
	protocol string
	// the protocol name used for Port.Name
	protocolName string
}

func ConvertServiceEntryMap(apps []*application, opts *convertOptions) (map[string]*networkingapi.ServiceEntry, error) {
	seMap := make(map[string]*networkingapi.ServiceEntry, 0)
	if len(apps) == 0 {
		return seMap, nil
	}
	for _, app := range apps {
		correctedAppName := strings.ReplaceAll(strings.ToLower(app.Name), "_", "-")
		if opts.appSuffix != "" {
			correctedAppName = correctedAppName + "." + opts.appSuffix
		}

		var projectCodes []string
		if opts.enableProjectCode {
			projectCodes = source.GetProjectCodeArr(app,
				func(a *application) []*instance { return a.Instances },
				func(i *instance) map[string]string { return i.Metadata },
			)
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

			for k, v := range convertServiceEntry(&projectApp, projectCode, opts) {
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
	app *application,
	projectCode string,
	opts *convertOptions,
) map[string]*networkingapi.ServiceEntry {
	nsEndpoints, nsSvcPorts, nsUseDnsMap := convertEndpointsWithNs(app.Instances, projectCode, opts)
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
		svcShortName = app.Name
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

func convertEndpointsWithNs(instances []*instance, projectCode string, opts *convertOptions,
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
		// if the project code is not empty, and the project code of the instance is not equal to the project code, skip it
		if projectCode != "" && ins.Metadata[source.ProjectCode] != projectCode {
			continue
		}

		metadata := ins.Metadata
		util.FilterEndpointLabels(metadata, opts.patchLabel, ins.IPAddress, "eureka:"+ins.InstanceID)

		var ns string
		if opts.nsHost {
			if v, ok := metadata["k8sNs"]; ok {
				ns = v
			} else {
				ns = opts.defaultSvcNs // "eureka" in old impl
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
			svcPortInUse = uint32(ins.Port.Port)
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
