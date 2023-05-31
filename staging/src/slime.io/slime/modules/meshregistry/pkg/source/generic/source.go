package generic

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "base")

const (
	ProjectCode = "projectCode"

	defaultServiceFilter = ""
)

type Instance[T any] interface {
	GetAddress() string
	GetInstanceID() string
	GetPort() int
	IsHealthy() bool
	GetMetadata() map[string]string
	MutableMetadata() *map[string]string
	Less(T) bool
	GetServiceName() string
	MutableServiceName() *string
}

type InstancesResp[I Instance[I], T any] interface {
	GetProjectCodes() []string
	GetInstances() []I
	GetDomain() string
	New(string, []I) T
}

type Client[I Instance[I], IR InstancesResp[I, IR]] interface {
	// Instances registered on the registery
	Instances() ([]IR, error)
}

type Option[I Instance[I], IR InstancesResp[I, IR]] func(s *Source[I, IR]) error

func WithDynamicConfigOption[I Instance[I], IR InstancesResp[I, IR]](addCb func(func(*bootstrap.SourceArgs))) Option[I, IR] {
	return func(s *Source[I, IR]) error {
		addCb(s.onConfig)
		return nil
	}
}

type Source[I Instance[I], IR InstancesResp[I, IR]] struct {
	// configuration
	args            *bootstrap.SourceArgs
	registry        string
	nsHost          bool
	k8sDomainSuffix bool

	// lifecycle management
	stop           chan struct{}
	seInitCh       chan struct{}
	started        bool
	initWg         sync.WaitGroup
	initedCallback func(string)
	delay          time.Duration

	// client
	client Client[I, IR]

	// xds
	cache             map[string]*networking.ServiceEntry
	seMergePortMocker *source.ServiceEntryMergePortMocker
	handlers          []event.Handler

	// dynamic config
	mut                   sync.RWMutex
	instanceFilter        func(I) bool
	reGroupInstances      func(in []IR) []IR
	serviceHostAliases    map[string][]string
	seMetaModifierFactory func(string) func(*resource.Metadata)
}

func NewSource[I Instance[I], IR InstancesResp[I, IR]](
	args *bootstrap.SourceArgs,
	registry string,
	nsHost, k8sDomainSuffix bool,
	delay time.Duration,
	readyCallback func(string),
	client Client[I, IR],
	options ...Option[I, IR],
) (*Source[I, IR], error) {
	var svcMocker *source.ServiceEntryMergePortMocker
	if args.MockServiceEntryName != "" {
		if args.MockServiceName == "" {
			return nil, fmt.Errorf("args MockServiceName empty but MockServiceEntryName %s", args.MockServiceEntryName)
		}
		svcMocker = source.NewServiceEntryMergePortMocker(
			args.MockServiceEntryName, args.ResourceNs, args.MockServiceName,
			args.MockServiceMergeInstancePort, args.MockServiceMergeServicePort,
			map[string]string{
				"registry": registry,
			})
	}
	ret := &Source[I, IR]{
		args:              args,
		registry:          registry,
		nsHost:            nsHost,
		k8sDomainSuffix:   k8sDomainSuffix,
		delay:             delay,
		client:            client,
		cache:             make(map[string]*networking.ServiceEntry),
		stop:              make(chan struct{}),
		seInitCh:          make(chan struct{}),
		seMergePortMocker: svcMocker,
	}
	if ret.seMergePortMocker != nil {
		ret.handlers = append(ret.handlers, ret.seMergePortMocker)
		svcMocker.SetDispatcher(func(meta resource.Metadata, item *networking.ServiceEntry) {
			ev := source.BuildServiceEntryEvent(event.Updated, item, meta)
			for _, h := range ret.handlers {
				h.Handle(ev)
			}
		})
	}

	ret.instanceFilter = generateInstanceFilter[I](args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll)
	ret.serviceHostAliases = generateServiceHostAliases(args.ServiceHostAliases)
	ret.seMetaModifierFactory = generateSeMetaModifierFactory(args.ServiceAdditionalMetas)
	ret.reGroupInstances = reGroupInstances[I, IR](args.InstanceMetaRelabel, args.ServiceNaming)

	for _, op := range options {
		if err := op(ret); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (s *Source[I, IR]) onConfig(args *bootstrap.SourceArgs) {
	var prevArgs *bootstrap.SourceArgs
	prevArgs, s.args = s.args, args

	s.mut.Lock()
	if !reflect.DeepEqual(prevArgs.EndpointSelectors, args.EndpointSelectors) ||
		!reflect.DeepEqual(prevArgs.ServicedEndpointSelectors, args.ServicedEndpointSelectors) {
		newInstSel := generateInstanceFilter[I](args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll)
		s.instanceFilter = newInstSel
	}

	if !reflect.DeepEqual(prevArgs.ServiceHostAliases, args.ServiceHostAliases) {
		newSvcHostAliases := generateServiceHostAliases(args.ServiceHostAliases)
		s.serviceHostAliases = newSvcHostAliases
	}

	if !reflect.DeepEqual(prevArgs.ServiceAdditionalMetas, args.ServiceAdditionalMetas) {
		newSeModifierFactory := generateSeMetaModifierFactory(args.ServiceAdditionalMetas)
		s.seMetaModifierFactory = newSeModifierFactory
	}

	if !reflect.DeepEqual(prevArgs.InstanceMetaRelabel, args.InstanceMetaRelabel) ||
		!reflect.DeepEqual(prevArgs.ServiceNaming, args.ServiceNaming) {
		newReGroupInstances := reGroupInstances[I, IR](args.InstanceMetaRelabel, args.ServiceNaming)
		s.reGroupInstances = newReGroupInstances
	}
	s.mut.Unlock()
}

func (s *Source[I, IR]) CacheJson(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(s.cache, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal %s se cache: %v", s.registry, err)
		return
	}
	_, _ = w.Write(b)
}

func (s *Source[I, IR]) Start() {
	s.initWg.Add(1)
	// if s.args.Mode == "polling" {
	go s.polling()
	// } else {
	// go s.watching()
	// }

	if s.seMergePortMocker != nil {
		s.initWg.Add(1)
		go func() {
			<-s.seInitCh

			log.Infof("[%%] service entry init done, begin to do init se merge port refresh")
			s.seMergePortMocker.Refresh()
			s.initWg.Done()

			s.seMergePortMocker.Start(nil)
		}()
	}

	if s.initedCallback != nil {
		go func() {
			s.initWg.Wait()
			s.initedCallback(s.registry)
		}()
	}
}

func (s *Source[I, IR]) Stop() {
	s.stop <- struct{}{}
}

func (s *Source[I, IR]) Dispatch(handler event.Handler) {
	if s.handlers == nil {
		s.handlers = make([]event.Handler, 0, 1)
	}
	s.handlers = append(s.handlers, handler)
}

func (s *Source[I, IR]) refresh() {
	if s.started {
		return
	}
	defer func() {
		s.started = false
	}()
	s.started = true
	log.Infof("%s refresh start : %d", s.registry, time.Now().UnixNano())
	s.updateServiceInfo()
	log.Infof("%s refresh finsh : %d", s.registry, time.Now().UnixNano())
	s.markServiceEntryInitDone()
}

func (s *Source[I, IR]) updateServiceInfo() {
	instances, err := s.client.Instances()
	if err != nil {
		log.Errorf("%s get instances failed: %v", s.registry, err)
		return
	}

	if s.reGroupInstances != nil {
		instances = s.reGroupInstances(instances)
	}

	newServiceEntryMap, err := convertServiceEntryMap(
		instances, s.registry, s.args.DefaultServiceNs, s.args.SvcPort, s.args.GatewayModel, s.nsHost, s.k8sDomainSuffix,
		s.args.InstancePortAsSvcPort, s.args.LabelPatch, false, s.getInstanceFilters(), s.getServiceHostAlias())
	if err != nil {
		log.Errorf("%s convert servceentry map failed: %s", s.registry, err.Error())
		return
	}

	seMetaModifierFactory := s.getSeMetaModifierFactory()

	for service, oldEntry := range s.cache {
		if _, ok := newServiceEntryMap[service]; !ok {
			// DELETE, set ep size to zero
			delete(s.cache, service)
			oldEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
			if event, err := buildEvent(event.Updated, oldEntry, "", service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
				log.Infof("delete(update) %s se, hosts: %s ,ep: %s ,size : %d ", s.registry, oldEntry.Hosts[0], printEps(oldEntry.Endpoints), len(oldEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			}
		}
	}
	for service, newEntry := range newServiceEntryMap {
		if oldEntry, ok := s.cache[service]; !ok {
			// ADD
			s.cache[service] = newEntry
			if event, err := buildEvent(event.Added, newEntry, "", service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
				log.Infof("add %s se, hosts: %s ,ep: %s, size: %d ", s.registry, newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			}
		} else {
			if !proto.Equal(oldEntry, newEntry) {
				// UPDATE
				s.cache[service] = newEntry
				if event, err := buildEvent(event.Updated, newEntry, "", service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
					log.Infof("update %s se, hosts: %s, ep: %s, size: %d ", s.registry, newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handlers {
						h.Handle(event)
					}
				}
			}
		}
	}

}

func (s *Source[I, IR]) markServiceEntryInitDone() {
	s.mut.RLock()
	ch := s.seInitCh
	s.mut.RUnlock()
	if ch == nil {
		return
	}

	s.mut.Lock()
	ch, s.seInitCh = s.seInitCh, nil
	s.mut.Unlock()
	if ch != nil {
		log.Infof("%s service entry init done, close ch and call initWg.Done", s.registry)
		s.initWg.Done()
		close(ch)
	}
}

func (s *Source[I, IR]) polling() {
	go func() {
		time.Sleep(s.delay)
		ticker := time.NewTicker(time.Duration(s.args.RefreshPeriod))
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.refresh()
			}
		}
	}()
}

func (s *Source[I, IR]) getInstanceFilters() func(I) bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.instanceFilter
}

func (s *Source[I, IR]) getServiceHostAlias() map[string][]string {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.serviceHostAliases
}

func (s *Source[I, IR]) getSeMetaModifierFactory() func(string) func(*resource.Metadata) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.seMetaModifierFactory
}

func convertServiceEntryMap[I Instance[I], IR InstancesResp[I, IR]](
	instances []IR,
	registry string,
	defaultSvcNs string,
	svcPort uint32,
	gatewayModel, nsfRegistry bool,
	nsHost, k8sDomainSuffix bool,
	patchLabel bool,
	instancePortAsSvcPort bool,
	filter func(I) bool,
	hostAliases map[string][]string,
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
func convertServiceEntry[I Instance[I], IR InstancesResp[I, IR]](app IR, registry string, nsHost bool, defaultSuffix string, patchLabel bool,
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
func convertServiceEntryWithProjectCode[I Instance[I], IR InstancesResp[I, IR]](app IR, registry string, nsHost bool, defaultSuffix string, patchLabel bool, projectCode string,
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
func convertServiceEntryWithNs[I Instance[I], IR InstancesResp[I, IR]](app IR, registry string, defaultNs string, svcPort uint32,
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

func buildEvent(
	kind event.Kind,
	item *networking.ServiceEntry,
	registry, service, resourceNs string,
	metaModifier func(meta *resource.Metadata)) (event.Event, error) {
	se := util.CopySe(item)
	items := strings.Split(service, ".")
	ns := resourceNs
	if len(items) > 1 {
		ns = items[1]
	}
	now := time.Now()
	meta := resource.Metadata{
		CreateTime: now,
		Labels: map[string]string{
			"registry": registry,
		},
		Version:     source.GenVersion(collections.K8SNetworkingIstioIoV1Alpha3Serviceentries),
		FullName:    resource.FullName{Name: resource.LocalName(service), Namespace: resource.Namespace(ns)},
		Annotations: map[string]string{},
	}
	if metaModifier != nil {
		metaModifier(&meta)
	}
	return source.BuildServiceEntryEvent(kind, se, meta), nil
}

func printEps(eps []*networking.WorkloadEntry) string {
	ips := make([]string, 0)
	for _, ep := range eps {
		ips = append(ips, ep.Address)
	}
	return strings.Join(ips, ",")
}

func generateInstanceFilter[I Instance[I]](
	svcSel map[string][]*metav1.LabelSelector, epSel []*metav1.LabelSelector, emptySelectorsReturn bool) func(I) bool {
	selectHookStore := source.NewSelectHookStore(svcSel, emptySelectorsReturn)
	selectHookStore[defaultServiceFilter] = source.NewSelectHook(epSel, emptySelectorsReturn)
	return func(i I) bool {
		filter := selectHookStore[i.GetServiceName()]
		if filter == nil {
			filter = selectHookStore[defaultServiceFilter]
		}
		return filter(i.GetMetadata())
	}
}

func generateServiceHostAliases(hostAliases []*bootstrap.ServiceHostAlias) map[string][]string {
	if len(hostAliases) != 0 {
		serviceHostAliases := make(map[string][]string, len(hostAliases))
		for _, ha := range hostAliases {
			serviceHostAliases[ha.Host] = ha.Aliases
		}
		return serviceHostAliases
	}
	return nil
}

func generateSeMetaModifierFactory(additionalMetas map[string]*bootstrap.MetadataWrapper) func(string) func(*resource.Metadata) {
	return func(s string) func(*resource.Metadata) {
		additionalMeta, exist := additionalMetas[s]
		if !exist || additionalMeta == nil {
			return func(_ *resource.Metadata) { /*do nothing*/ }
		}
		return func(m *resource.Metadata) {
			if len(additionalMeta.Labels) > 0 {
				if m.Labels == nil {
					m.Labels = make(resource.StringMap, len(additionalMeta.Labels))
				}
				for k, v := range additionalMeta.Labels {
					m.Labels[k] = v
				}
			}
			if len(additionalMeta.Annotations) > 0 {
				if m.Annotations == nil {
					m.Annotations = make(resource.StringMap, len(additionalMeta.Annotations))
				}
				for k, v := range additionalMeta.Annotations {
					m.Annotations[k] = v
				}
			}
		}
	}
}

func reGroupInstances[I Instance[I], IR InstancesResp[I, IR]](rl *bootstrap.InstanceMetaRelabel,
	c *bootstrap.ServiceNameConverter) func(in []IR) []IR {
	if rl == nil && c == nil {
		return nil
	}

	var instanceRelabel func(inst I) = func(inst I) { /*do nothing*/ }
	if rl != nil {
		var relabelFuncs []func(inst I)
		for _, relabel := range rl.Items {
			f := func(item *bootstrap.InstanceMetaRelabelItem) func(inst I) {
				return func(inst I) {
					mutMeta := inst.MutableMetadata()
					if len(*mutMeta) == 0 ||
						item.Key == "" || item.TargetKey == "" {
						return
					}
					v, exist := (*mutMeta)[item.Key]
					if !exist {
						return
					} else {
						if nv, exist := item.ValuesMapping[v]; exist {
							v = nv
						}
					}
					if _, exist := (*mutMeta)[item.TargetKey]; !exist || item.Overwirte {
						(*mutMeta)[item.TargetKey] = v
					}
				}
			}(relabel)
			relabelFuncs = append(relabelFuncs, f)
		}
		instanceRelabel = func(inst I) {
			for _, f := range relabelFuncs {
				f(inst)
			}
		}
	}

	var instanceDom func(inst I) string = func(inst I) string { return inst.GetServiceName() }
	if c != nil {
		var substrFuncs []func(inst I) string
		for _, item := range c.Items {
			var substrF func(inst I) string
			switch item.Kind {
			case bootstrap.InstanceBasicInfoKind:
				switch item.Value {
				case bootstrap.InstanceBasicInfoSvc:
					substrF = func(inst I) string { return inst.GetServiceName() }
				case bootstrap.InstanceBasicInfoIP:
					substrF = func(inst I) string { return inst.GetAddress() }
				case bootstrap.InstanceBasicInfoPort:
					substrF = func(inst I) string { return fmt.Sprintf("%d", inst.GetPort()) }
				}
			case bootstrap.InstanceMetadataKind:
				substrF = func(meta string) func(inst I) string {
					return func(inst I) string {
						if inst.GetMetadata() == nil {
							return ""
						}
						return inst.GetMetadata()[meta]
					}
				}(item.Value)
			case bootstrap.StaticKind:
				substrF = func(staticValue string) func(_ I) string {
					return func(_ I) string { return staticValue }
				}(item.Value)
			}
			substrFuncs = append(substrFuncs, substrF)
		}
		instanceDom = func(inst I) string {
			subs := make([]string, 0, len(c.Items))
			for _, f := range substrFuncs {
				subs = append(subs, f(inst))
			}
			svcName := strings.Join(subs, c.Sep)
			// overwrite the original service name
			*inst.MutableServiceName() = svcName
			return svcName
		}
	}

	return func(in []IR) []IR {
		m := map[string][]I{}
		for _, ir := range in {
			for _, inst := range ir.GetInstances() {
				instanceRelabel(inst)
				dom := instanceDom(inst)
				m[dom] = append([]I(m[dom]), inst)
			}
		}
		out := make([]IR, 0, len(m))
		for dom, insts := range m {
			ir := new(IR)
			out = append(out, (*ir).New(dom, insts))
		}
		return out
	}
}
