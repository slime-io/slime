package zookeeper

import (
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	networkingapi "istio.io/api/networking/v1alpha3"

	"slime.io/pkg/text"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

const (
	schemeDubbo    = "dubbo://"
	schemeConsumer = "consumer://"

	dubboSvcAppLabel      = "application"
	dubboSvcMethodEqLabel = "istio.io/dubbomethodequal"

	dubboParamInterfaceKey      = "interface"
	dubboParamGroupKey          = "group"
	dubboParamDefaultGroupKey   = "default.group"
	dubboParamVersionKey        = "version"
	dubboParamDefaultVersionKey = "default.version"
	dubboParamMethods           = "methods"
	dubboTag                    = "dubbo.tag"

	metaDataServiceKey            = "dubbo.metadata-service.url-params"
	metadataServicePortNamePrefix = "metadata-service-port-"
	dubboServiceKeySep            = ":"
)

type Error struct {
	msg string
}

func (e Error) Error() string {
	return e.msg
}

// trimSameDubboMethodsLabel removes the dubbo methods label when all endpoints have same methods because in this case
// envoy doesn't need to do the method-lb among the eps and therefore this info is not required.
// Also, not-send-ep-methods-to-envoy can greatly reduce the perf overhead in envoy side.
// NOTE: if the same-dubbo-methods state keeps flipping, it will bring frequent full-pushes in istio side. But we do not
// care about this rare case.
func trimSameDubboMethodsLabel(se *networkingapi.ServiceEntry) bool {
	var (
		prev string
		diff bool
	)
	for idx, ep := range se.Endpoints {
		epMethods := ep.Labels[dubboParamMethods]
		if epMethods != prev && idx > 0 {
			diff = true
			break
		}
		prev = epMethods
	}

	if !diff {
		for _, ep := range se.Endpoints {
			if ep.Labels != nil {
				delete(ep.Labels, dubboParamMethods)
			}
		}
	}

	return !diff
}

func getEndpointsLabel(endpoints []*networkingapi.WorkloadEntry, key string, skipEmpty bool) string {
	for _, ep := range endpoints {
		if v, ok := ep.GetLabels()[key]; ok {
			if skipEmpty && v == "" {
				continue
			}
			return v
		}
	}
	return ""
}

type convertedServiceEntry struct {
	se               *networkingapi.ServiceEntry
	methodsLabel     string
	labels           map[string]string
	InboundEndPoints []*networkingapi.WorkloadEntry
}

type convertOptions struct {
	patchLabel            bool
	instancePortAsSvcPort bool
	svcPort               uint32
	hostSuffix            string
	ignoreLabels          map[string]string
	filter                func(*dubboInstance) bool

	// the protocol used for Port.Protocol
	protocol string
	// the protocol name used for Port.Name
	protocolName string
}

func (s *Source) convertServiceEntry(
	providers, consumers, configurators []string, service string, opts *convertOptions,
) (map[string][]dubboInstance, map[string]*convertedServiceEntry) {
	registryServicesByServiceKey := make(map[string][]dubboInstance)
	serviceEntryByServiceKey := make(map[string]*convertedServiceEntry)
	methodsByServiceKey := make(map[string]map[string]struct{})

	defer func() {
		for k, cse := range serviceEntryByServiceKey {
			cse.labels = map[string]string{}
			if s.args.SingleAppService {
				cse.labels[dubboSvcAppLabel] = getEndpointsLabel(cse.se.Endpoints, dubboSvcAppLabel, false)
			}

			var enableMethodLB bool
			s.mut.RLock()
			methodLBChecker := s.methodLBChecker
			s.mut.RUnlock()
			if methodLBChecker != nil {
				enableMethodLB = methodLBChecker(cse)
			}

			if !trimSameDubboMethodsLabel(cse.se) && enableMethodLB {
				// to trigger svc change/full push in istio sidecar when eq -> uneq or uneq -> eq
				cse.labels[dubboSvcMethodEqLabel] = strconv.FormatBool(false)
			}
			if v := methodsByServiceKey[k]; len(v) > 0 {
				methods := make([]string, 0, len(v))
				for method := range v {
					methods = append(methods, method)
				}
				sort.Strings(methods)

				cse.methodsLabel = text.EscapeLabelValues(methods)
			}
		}
	}()

	if len(providers) == 0 {
		log.Debugf("%s no provider", service)
		return registryServicesByServiceKey, serviceEntryByServiceKey
	}

	var extraMeta map[configuratorMetaKey]map[string]string
	if len(configurators) > 0 && defaultExtraMetaPatcher != nil {
		extraMeta = defaultExtraMetaPatcher.parseExtraMeta(configurators, service)
	}

	uniquePort := make(map[string]map[uint32]struct{})

	for _, provider := range providers {
		providerParts := splitUrl(provider)
		if providerParts == nil {
			continue
		}

		addr, portNum, err := parseAddr(providerParts[2])
		if err != nil {
			log.Errorf("invalid provider ip or port %s of %s", provider, service)
			continue
		}

		var (
			methods       = map[string]struct{}{}
			methodApplier = func(method string) {
				methods[method] = struct{}{}
			}
		)

		// extract dubbo metadata and methods from url
		meta, ok := verifyMeta(providerParts[len(providerParts)-1], addr, portNum, service, opts.patchLabel, opts.ignoreLabels, methodApplier, extraMeta) //nolint: lll
		if !ok {
			continue
		}
		if s.instanceMetaModifier != nil {
			s.instanceMetaModifier(&meta)
		}
		serviceKey := buildServiceKey(service, meta) // istio service host

		// now we have the necessary info to build the dubboinstance,
		// so we can filter out the instance if needed
		instance := dubboInstance{
			Addr:     addr,
			Port:     portNum,
			Service:  service,
			Metadata: meta,
		}
		registryServicesByServiceKey[serviceKey] = append(registryServicesByServiceKey[serviceKey], instance)
		if opts.filter != nil && !opts.filter(&instance) {
			continue
		}

		// update methods of the service
		if len(methods) > 0 {
			serviceMethods := methodsByServiceKey[serviceKey]
			if serviceMethods == nil {
				methodsByServiceKey[serviceKey] = methods
			} else {
				for method := range methods {
					serviceMethods[method] = struct{}{}
				}
			}
		}

		// get or create convertedServiceEntry
		cse := serviceEntryByServiceKey[serviceKey]
		if cse == nil {
			se := &networkingapi.ServiceEntry{
				Ports: make([]*networkingapi.ServicePort, 0),
			}
			cse = &convertedServiceEntry{se: se}
			serviceEntryByServiceKey[serviceKey] = cse
			// append suffix to service host if needed
			host := serviceKey
			if opts.hostSuffix != "" {
				host += opts.hostSuffix
			}
			se.Hosts = []string{host}
			se.Resolution = networkingapi.ServiceEntry_STATIC

			// try build inbound endpoints
			for _, consumer := range consumers {
				consumerParts := splitUrl(consumer)
				if consumerParts == nil {
					continue
				}

				cAddr := consumerParts[2]
				if idx := strings.Index(cAddr, ":"); idx >= 0 { // consumer url generally does not have port
					addr, _, err := parseAddr(cAddr)
					if err != nil {
						// missing consumer port is not that critical, thus we continue to ...
						log.Debugf("invalid consumer %s of %s", consumer, serviceKey)
						cAddr = cAddr[:idx]
					} else {
						cAddr = addr
					}
				}

				// XXX optimize inbound ep meta calculation
				if meta, ok := verifyMeta(consumerParts[len(consumerParts)-1], cAddr, portNum, "", opts.patchLabel, opts.ignoreLabels, nil, nil); ok { //nolint: lll
					meta = consumerMeta(meta)
					consumerServiceKey := buildServiceKey(service, meta)
					if consumerServiceKey == serviceKey {
						cse.InboundEndPoints = append(cse.InboundEndPoints, convertInboundEndpoint(cAddr, meta))
					}
				}
			}
		}
		se := cse.se

		svcPortInUse := opts.svcPort
		if opts.instancePortAsSvcPort {
			svcPortInUse = portNum
		}

		// add endpoint
		se.Endpoints = append(se.Endpoints,
			convertEndpoint(addr, meta, &networkingapi.ServicePort{
				Number:   portNum,
				Protocol: opts.protocol,
				Name:     source.PortName(opts.protocolName, svcPortInUse),
			}),
		)

		// update ports of the serviceEntry
		if _, ok := uniquePort[serviceKey]; !ok {
			uniquePort[serviceKey] = make(map[uint32]struct{})
		}
		svcPortsToAdd := []uint32{svcPortInUse}
		if opts.svcPort != 0 && opts.svcPort != svcPortInUse {
			svcPortsToAdd = append(svcPortsToAdd, opts.svcPort)
		}
		for _, p := range svcPortsToAdd {
			if _, ok := uniquePort[serviceKey][p]; !ok {
				se.Ports = append(se.Ports, &networkingapi.ServicePort{
					Number:   p,
					Protocol: opts.protocol,
					Name:     source.PortName(opts.protocolName, p),
				})
				uniquePort[serviceKey][p] = struct{}{}
			}
		}
	}

	for _, cse := range serviceEntryByServiceKey {
		source.ApplyServicePortToEndpoints(cse.se)
		source.RectifyServiceEntry(cse.se)
	}

	return registryServicesByServiceKey, serviceEntryByServiceKey
}

func extractGroup(meta map[string]string) string {
	g := meta[dubboParamGroupKey]
	if len(g) == 0 {
		g = meta[dubboParamDefaultGroupKey]
	}
	return g
}

func extractVersion(meta map[string]string) string {
	v := meta[dubboParamVersionKey]
	if len(v) == 0 {
		v = meta[dubboParamDefaultVersionKey]
	}
	return v
}

func buildServiceKey(service string, meta map[string]string) string {
	group, version := extractGroup(meta), extractVersion(meta)
	parts := []string{service, group, version}
	// trim trailing empty parts
	i := len(parts) - 1
	for ; i >= 0; i-- {
		if parts[i] != "" {
			break
		}
	}
	parts = parts[:i+1]
	// NOTE: this hostname format will be used in istiod. Any change should be syned with that.
	return strings.Join(parts, dubboServiceKeySep)
}

func parseServiceFromKey(serviceKey string) string {
	idx := strings.Index(serviceKey, dubboServiceKeySep)
	if idx < 0 {
		return serviceKey
	}
	return serviceKey[:idx]
}

func parseAddr(addr string) (ip string, port uint32, err error) {
	addrParts := strings.Split(addr, ":")
	if len(addrParts) != 2 {
		err = util.ErrValue
		return
	}

	if net.ParseIP(addrParts[0]) == nil {
		err = util.ErrValue
		return
	}
	ip = addrParts[0]

	if v, curErr := strconv.Atoi(addrParts[1]); curErr != nil {
		err = curErr
		return
	} else if v > math.MaxUint16 || v <= 0 {
		// istio port定义为int32，xds定义将范围变小 -->   uint32 port_value = 1 [(validate.rules).uint32.lte = 65535];
		err = util.ErrValue
		return
	} else {
		port = uint32(v)
	}

	return
}

func consumerMeta(labels map[string]string) map[string]string {
	for k := range labels {
		switch k {
		case dubboSvcAppLabel, // consumer's app label
			dubboParamInterfaceKey, // provider's own interface/group/version
			dubboParamGroupKey, dubboParamDefaultGroupKey,
			dubboParamVersionKey, dubboParamDefaultVersionKey:
			continue
		default:
			delete(labels, k)
		}
	}
	return labels
}

// XXX aggregate the parameters
func verifyMeta(url string, ip string, port uint32, service string, patchLabel bool, ignoreLabels map[string]string,
	methodApplier func(string), extraMeta map[configuratorMetaKey]map[string]string,
) (map[string]string, bool) {
	if !strings.Contains(url, "?") {
		log.Errorf("Invaild dubbo url, missing '?' %s", url)
		return nil, false
	}
	metaStr := url[strings.Index(url, "?")+1:]
	entries := strings.Split(metaStr, "&")
	meta := make(map[string]string, len(entries))
	for _, entry := range entries {
		kv := strings.SplitN(entry, "=", 2)
		if len(kv) != 2 {
			log.Errorf("Invaild dubbo url, invaild meta info : %s", entry)
			return nil, false
		}
		k, v := kv[0], kv[1]
		if _, exist := ignoreLabels[k]; exist {
			continue
		}

		switch k {
		case dubboTag:
			parseDubboTag(v, meta)
			continue
		case dubboParamMethods:
			methods := strings.Split(v, ",")
			sort.Strings(methods)
			if methodApplier != nil {
				for _, method := range methods {
					if method != "" {
						methodApplier(method)
					}
				}
			}
			meta[k] = text.EscapeLabelValues(methods)
			continue
		}

		v = strings.ReplaceAll(v, ",", "_")
		v = strings.ReplaceAll(v, ":", "_")
		meta[k] = v
	}

	if extraMeta != nil && defaultExtraMetaPatcher != nil {
		metaKey := configuratorMetaKey{
			ip:      ip,
			port:    port,
			service: service,
			group:   extractGroup(meta),
			version: extractVersion(meta),
		}
		defaultExtraMetaPatcher.patchExtraMeta(metaKey, meta, extraMeta)
	}

	util.FilterEndpointLabels(meta, patchLabel, ip, "zookeeper:"+ip)
	return meta, true
}

func parseDubboTag(str string, meta map[string]string) {
	pairs := strings.Split(str, ",")
	if len(pairs) == 1 {
		meta["dubboTag"] = pairs[0]
	} else {
		for _, v := range pairs {
			kv := strings.Split(v, "=")
			if len(kv) != 2 {
				log.Errorf("Invaild dubbo tag %s", v)
				continue
			}
			meta[kv[0]] = kv[1]
		}
	}
}

func convertEndpoint(ip string, meta map[string]string, port *networkingapi.ServicePort) *networkingapi.WorkloadEntry {
	ret := &networkingapi.WorkloadEntry{
		Address: ip,
		Ports: map[string]uint32{
			port.Name: port.Number,
		},
		Labels: meta,
	}

	util.FillWorkloadEntryLocality(ret)

	return ret
}

type InboundEndPoint struct {
	Address string
	Labels  map[string]string
	Ports   map[string]uint32
}

func convertInboundEndpoint(ip string, meta map[string]string) *networkingapi.WorkloadEntry {
	inboundEndpoint := &networkingapi.WorkloadEntry{
		Address: ip,
		Labels:  meta,
	}
	return inboundEndpoint
}

func splitUrl(zkChild string) []string {
	path, err := url.PathUnescape(zkChild)
	if err != nil {
		log.Errorf(err.Error())
		return nil
	}
	if !strings.HasPrefix(path, schemeDubbo) && !strings.HasPrefix(path, schemeConsumer) {
		return nil
	}
	ss := strings.SplitN(path, "/", 4)
	if len(ss) < 4 {
		log.Errorf("Invaild dubbo Url：%s", zkChild)
		return nil
	}
	return ss
}

type configuratorMetaKey struct {
	ip   string
	port uint32

	// dubbo interface、group、version
	service string
	group   string
	version string
}

type configuratorMetaPatcher interface {
	parseExtraMeta(configurators []string, dubboInterface string) map[configuratorMetaKey]map[string]string
	patchExtraMeta(key configuratorMetaKey, meta map[string]string, extraMeta map[configuratorMetaKey]map[string]string)
}

var defaultExtraMetaPatcher configuratorMetaPatcher
