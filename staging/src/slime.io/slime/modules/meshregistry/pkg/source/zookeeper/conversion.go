package zookeeper

import (
	"bytes"
	"encoding/json"
	"math"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	networking "istio.io/api/networking/v1alpha3"

	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

const (
	Dubbo                        = "dubbo://"
	Consumer                     = "consumer://"
	NetworkProtocolDubbo         = "dubbo" // XXX change to DUBBO
	DubboPortName                = "dubbo"
	DubboServiceModelInterface   = "interface"
	DubboServiceModelApplication = "application"
	DubboServiceKeySep           = ":"
)

const (
	dubboParamGroupKey            = "group"
	dubboParamDefaultGroupKey     = "default.group"
	dubboParamVersionKey          = "version"
	dubboParamDefaultVersionKey   = "default.version"
	dubboTag                      = "dubbo.tag"
	dubboParamMethods             = "methods"
	metaDataServiceKey            = "dubbo.metadata-service.url-params"
	metadataServicePortNamePrefix = "metadata-service-port-"
	DubboHostnameSuffix           = ".dubbo"
	DubboSvcAppLabel              = "application"
	DubboSvcMethodEqLabel         = "istio.io/dubbomethodequal"
)

const (
	dns1123LabelMaxLength int    = 63
	dns1123LabelFmt       string = "[a-zA-Z0-9]([-a-z-A-Z0-9]*[a-zA-Z0-9])?"
	// a wild-card prefix is an '*', a normal DNS1123 label with a leading '*' or '*-', or a normal DNS1123 label
	wildcardPrefix = `(\*|(\*|\*-)?` + dns1123LabelFmt + `)`

	// Using kubernetes requirement, a valid key must be a non-empty string consist
	// of alphanumeric characters, '-', '_' or '.', and must start and end with an
	// alphanumeric character (e.g. 'MyValue',  or 'my_value',  or '12345'
	qualifiedNameFmt string = "([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]"
)

var wildcardPrefixRegexp = regexp.MustCompile("^" + wildcardPrefix + "$")

type Error struct {
	msg string
}

func (e Error) Error() string {
	return e.msg
}

func getMetaServicePort(payload ServiceInstancePayload) *networking.Port {
	if payload.Metadata == nil || len(payload.Metadata) == 0 || payload.Metadata[metaDataServiceKey] == "" {
		return nil
	}
	urlParam := payload.Metadata[metaDataServiceKey]

	urlParamMap := make(map[string]map[string]string)
	if err := json.Unmarshal([]byte(urlParam), &urlParamMap); err == nil {
		metadataServiceParam := urlParamMap[DubboPortName]
		if port, ok := metadataServiceParam["port"]; ok {
			if portNum, err := strconv.Atoi(port); err == nil {
				portName := metadataServicePortNamePrefix + port
				metaServicePort := &networking.Port{
					Number:   uint32(portNum),
					Protocol: NetworkProtocolDubbo,
					Name:     portName,
				}
				return metaServicePort
			}
		}
	}
	return nil
}

func verifyServiceInstance(si DubboServiceInstance) bool {
	if net.ParseIP(si.Address) == nil {
		return false
	}
	if si.Name == "" {
		return false
	}
	if si.Payload.Metadata == nil || si.Payload.Metadata[metaDataServiceKey] == "" {
		return false
	}
	return true
}

// trimSameDubboMethodsLabel removes the dubbo methods label when all endpoints have same methods because in this case
// envoy doesn't need to do the method-lb among the eps and therefore this info is not required.
// Also, not-send-ep-methods-to-envoy can greatly reduce the perf overhead in envoy side.
// NOTE: if the same-dubbo-methods state keeps flipping, it will bring frequent full-pushes in istio side. But we do not
// care about this rare case.
func trimSameDubboMethodsLabel(se *networking.ServiceEntry) bool {
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

type convertedServiceEntry struct {
	se               *networking.ServiceEntry
	methodsEqual     bool
	methodsLabel     string
	InboundEndPoints []*networking.WorkloadEntry
}

func convertServiceEntry(
	providers, consumers []string, service string, svcPort uint32, instancePortAsSvcPort, patchLabel bool,
	ignoreLabels map[string]string, gatewayMode bool) map[string]*convertedServiceEntry {
	// TODO y: sort endpoints
	serviceEntryByServiceKey := make(map[string]*convertedServiceEntry)
	methodsByServiceKey := make(map[string]map[string]struct{})
	defer func() {
		for k, cse := range serviceEntryByServiceKey {
			cse.methodsEqual = trimSameDubboMethodsLabel(cse.se)
			if v := methodsByServiceKey[k]; len(v) > 0 {
				methods := make([]string, 0, len(v))
				for method := range v {
					methods = append(methods, method)
				}
				sort.Strings(methods)

				cse.methodsLabel = escapeDubboMethods(methods, nil)
			}
		}
	}()
	if providers == nil || len(providers) == 0 {
		log.Debugf("%s no provider", service)
		return serviceEntryByServiceKey
	}

	uniquePort := make(map[string]map[uint32]struct{})

	// 获取provider 信息
	for _, provider := range providers {
		providerParts := splitUrl(provider)
		if providerParts == nil {
			continue
		}

		svcPortInUse := svcPort
		addr, portNum, err := parseAddr(providerParts[2])
		if err != nil {
			log.Errorf("invalid provider ip or port %s of %s", provider, service)
			continue
		}
		if instancePortAsSvcPort {
			svcPortInUse = portNum
		}
		instPort := convertPort(svcPortInUse, portNum)

		methods := map[string]struct{}{}
		meta, ok := verifyMeta(providerParts[len(providerParts)-1], addr, patchLabel, ignoreLabels, func(method string) {
			methods[method] = struct{}{}
		})
		if !ok {
			continue
		}

		serviceKey := buildServiceKey(service, meta)
		if len(methods) > 0 {
			serviceMethods := methodsByServiceKey[serviceKey]
			if serviceMethods == nil {
				serviceMethods = methods
				methodsByServiceKey[serviceKey] = methods
			} else {
				for method := range methods {
					serviceMethods[method] = struct{}{}
				}
			}
		}

		cse := serviceEntryByServiceKey[serviceKey]
		if cse == nil {
			se := &networking.ServiceEntry{
				Ports: make([]*networking.Port, 0),
			}
			cse = &convertedServiceEntry{se: se}
			serviceEntryByServiceKey[serviceKey] = cse
			// XXX 网关模式下，服务host添加".dubbo"后缀
			if gatewayMode {
				se.Hosts = []string{serviceKey + DubboHostnameSuffix}
			} else {
				se.Hosts = []string{serviceKey}
			}
			se.Resolution = networking.ServiceEntry_STATIC

			for _, consumer := range consumers {
				consumerParts := splitUrl(consumer)
				if consumerParts == nil {
					continue
				}

				var (
					cAddr = consumerParts[2]
					cPort *networking.Port
				)

				if idx := strings.Index(cAddr, ":"); idx >= 0 { // consumer url generally does not have port
					addr, portNum, err := parseAddr(cAddr)
					if err != nil {
						// missing consumer port is not that critical, thus we continue to ...
						log.Debugf("invalid consumer %s of %s", consumer, serviceKey)
						cAddr = cAddr[:idx]
					} else {
						cPort = convertPort(portNum, portNum)
						cAddr = addr
					}
				}

				if meta, ok := verifyMeta(consumerParts[len(consumerParts)-1], cAddr, patchLabel, ignoreLabels, nil); ok {
					meta = consumerMeta(meta)
					consumerServiceKey := buildServiceKey(service, meta)
					if consumerServiceKey == serviceKey {
						cse.InboundEndPoints = append(cse.InboundEndPoints, convertInboundEndpoint(cAddr, meta, cPort))
					}
				}
			}
		}
		se := cse.se

		se.Endpoints = append(se.Endpoints, convertEndpoint(addr, meta, instPort))

		if _, ok := uniquePort[serviceKey]; !ok {
			uniquePort[serviceKey] = make(map[uint32]struct{})
		}

		svcPortsToAdd := []uint32{svcPortInUse}
		if svcPort != 0 && svcPort != svcPortInUse {
			svcPortsToAdd = append(svcPortsToAdd, svcPort)
		}
		for _, p := range svcPortsToAdd {
			if _, ok := uniquePort[serviceKey][p]; !ok {
				se.Ports = append(se.Ports, convertPort(p, p))
				uniquePort[serviceKey][p] = struct{}{}
			}
		}
	}

	for _, cse := range serviceEntryByServiceKey {
		source.ApplyServicePortToEndpoints(cse.se)
		source.RectifyServiceEntry(cse.se)
	}

	return serviceEntryByServiceKey
}

func buildServiceKey(service string, meta map[string]string) string {
	group := meta[dubboParamGroupKey]
	if len(group) == 0 {
		group = meta[dubboParamDefaultGroupKey]
	}
	version := meta[dubboParamVersionKey]
	if len(version) == 0 {
		version = meta[dubboParamDefaultVersionKey]
	}

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
	return strings.Join(parts, DubboServiceKeySep)
}

func parseServiceFromKey(serviceKey string) string {
	idx := strings.Index(serviceKey, DubboServiceKeySep)
	if idx < 0 {
		return serviceKey
	}
	return serviceKey[:idx]
}

func parseAddr(addr string) (ip string, port uint32, err error) {
	addrParts := strings.Split(addr, ":")
	if len(addrParts) != 2 {
		err = util.ValueError
		return
	}

	if net.ParseIP(addrParts[0]) == nil {
		err = util.ValueError
		return
	} else {
		ip = addrParts[0]
	}

	if v, curErr := strconv.Atoi(addrParts[1]); curErr != nil {
		err = curErr
		return
	} else if v > math.MaxUint16 || v <= 0 {
		// istio port定义为int32，xds定义将范围变小 -->   uint32 port_value = 1 [(validate.rules).uint32.lte = 65535];
		err = util.ValueError
		return
	} else {
		port = uint32(v)
	}

	return
}

func consumerMeta(labels map[string]string) map[string]string {
	for k := range labels {
		if k != "application" && k != "interface" && k != "side" && k != "group" && k != "version" {
			delete(labels, k)
		}
	}
	return labels
}

func verifyMeta(url string, ip string, patchLabel bool, ignoreLabels map[string]string, methodApplier func(string)) (map[string]string, bool) {
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
			meta[k] = escapeDubboMethods(methods, methodApplier)
			continue
		}

		v = strings.ReplaceAll(v, ",", "_")
		v = strings.ReplaceAll(v, ":", "_")
		meta[k] = v
		/*		if wildcardPrefixRegexp.MatchString(kv[1]) {

				} else {
					log.Warnf("invalid tag value: %s", kv[1])
				}*/
	}
	util.FilterLabels(meta, patchLabel, ip, "zookeeper:"+ip)
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

func convertPort(svcPort, port uint32) *networking.Port {
	return &networking.Port{
		Protocol: NetworkProtocolDubbo,
		Number:   port,
		Name:     source.PortName(NetworkProtocolDubbo, svcPort),
	}
}

func convertEndpoint(ip string, meta map[string]string, port *networking.Port) *networking.WorkloadEntry {
	ret := &networking.WorkloadEntry{
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

func convertInboundEndpoint(ip string, meta map[string]string, port *networking.Port) *networking.WorkloadEntry {
	inboundEndpoint := &networking.WorkloadEntry{
		Address: ip,
		Labels:  meta,
	}
	if port != nil {
		inboundEndpoint.Ports = map[string]uint32{
			port.Name: port.Number,
		}
	}
	return inboundEndpoint
}

func splitUrl(zkChild string) []string {
	path, err := url.PathUnescape(zkChild)
	if err != nil {
		log.Errorf(err.Error())
		return nil
	}
	if !strings.HasPrefix(path, Dubbo) && !strings.HasPrefix(path, Consumer) {
		return nil
	}
	ss := strings.SplitN(path, "/", 4)
	if len(ss) < 4 {
		log.Errorf("Invaild dubbo Url：%s", zkChild)
		return nil
	}
	return ss
}

// escapeDubboMethods caller should ensure that methods is an ordered list
func escapeDubboMethods(methods []string, methodApplier func(string)) string {
	// printData2,printData1$ -> printData12-4.printData2 while hex('$') == "24"
	isValidChar := func(b byte) bool {
		if 'a' <= b && b <= 'z' {
			return true
		}
		if 'A' <= b && b <= 'Z' {
			return true
		}
		if '0' <= b && b <= '9' {
			return true
		}
		if '_' == b {
			return true
		}
		return false
	}

	const (
		hextable  = "0123456789abcdef"
		sep       = '-'
		methodSep = '.'
	)

	buf := &bytes.Buffer{}
	for _, method := range methods {
		if method == "" {
			continue
		}

		if methodApplier != nil {
			methodApplier(method)
		}

		for idx := 0; idx < len(method); idx++ {
			c := method[idx]
			if isValidChar(c) {
				buf.WriteByte(c)
			} else {
				buf.WriteByte(hextable[c>>4])
				buf.WriteByte(sep)
				buf.WriteByte(hextable[c&0x0f])
			}
		}
		buf.WriteByte(methodSep)
	}

	ret := buf.String()
	if l := len(ret); l > 0 && ret[l-1] == methodSep {
		// remove trailing sep
		ret = ret[:l-1]
	}
	return ret
}
