package nacos

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	cmap "github.com/orcaman/concurrent-map/v2"
	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "nacos")

type Source struct {
	args *bootstrap.NacosSourceArgs // should only be accessed in `onConfig`

	// nacos client
	client            Client
	namingClient      naming_client.INamingClient
	seMergePortMocker *source.ServiceEntryMergePortMocker

	// common configs
	delay time.Duration

	// source cache
	cache             map[string]*networkingapi.ServiceEntry
	namingServiceList cmap.ConcurrentMap[string, bool]
	handlers          []event.Handler

	mut sync.RWMutex

	// source status
	started  bool
	stop     chan struct{}
	seInitCh chan struct{}
	initWg   sync.WaitGroup

	initedCallback func(string)

	// instanceFiler fitler which instance of a service should be include
	// Updates are only allowed when the configuration is loaded or reloaded.
	instanceFilter func(*instance) bool

	reGroupInstances func(in []*instanceResp) []*instanceResp

	// serviceHostAliases, the key of the map is the original host of a service, and
	// if an original host exists in serviceHostAliases, the corresponding value will
	// be appended to the converted ServiceEntry hosts.
	// Updates are only allowed when the configuration is loaded or reloaded.
	serviceHostAliases    map[string][]string
	seMetaModifierFactory func(string) func(*resource.Metadata)
}

const (
	SourceName       = "nacos"
	HttpPath         = "/nacos"
	POLLING          = "polling"
	WATCHING         = "watching"
	clientHeadersEnv = "NACOS_CLIENT_HEADERS"

	defaultServiceFilter = ""
)

type Option func(s *Source) error

func WithDynamicConfigOption(addCb func(func(*bootstrap.NacosSourceArgs))) Option {
	return func(s *Source) error {
		addCb(s.onConfig)
		return nil
	}
}

func New(args *bootstrap.NacosSourceArgs, nsHost bool, k8sDomainSuffix bool, delay time.Duration, readyCallback func(string), options ...Option) (event.Source, func(http.ResponseWriter, *http.Request), error) {
	var svcMocker *source.ServiceEntryMergePortMocker
	if args.MockServiceEntryName != "" {
		if args.MockServiceName == "" {
			return nil, nil, fmt.Errorf("args MockServiceName empty but MockServiceEntryName %s", args.MockServiceEntryName)
		}
		svcMocker = source.NewServiceEntryMergePortMocker(
			args.MockServiceEntryName, args.ResourceNs, args.MockServiceName,
			args.MockServiceMergeInstancePort, args.MockServiceMergeServicePort,
			map[string]string{
				"registry": SourceName,
			})
	}

	ret := &Source{
		args:              args,
		delay:             delay,
		started:           false,
		initedCallback:    readyCallback,
		cache:             make(map[string]*networkingapi.ServiceEntry),
		namingServiceList: cmap.New[bool](),
		stop:              make(chan struct{}),
		seInitCh:          make(chan struct{}),
		seMergePortMocker: svcMocker,
	}

	headers := make(map[string]string)
	nacosHeaders := os.Getenv(clientHeadersEnv)
	if nacosHeaders != "" {
		for _, header := range strings.Split(nacosHeaders, ",") {
			items := strings.SplitN(header, "=", 2)
			if len(items) == 2 {
				headers[items[0]] = items[1]
			}
		}
	}

	if args.Mode == POLLING {
		servers := args.Servers
		if len(servers) == 0 {
			servers = []bootstrap.NacosServer{args.NacosServer}
		}
		ret.client = NewClients(servers, args.MetaKeyNamespace, args.MetaKeyGroup, headers)
	} else {
		namingClient, err := newNamingClient(args.Address, args.Namespace, headers)
		if err != nil {
			return nil, nil, Error{
				msg: fmt.Sprintf("init nacos client failed: %s", err.Error()),
			}
		}
		ret.namingClient = namingClient
	}

	ret.initWg.Add(1)
	if ret.seMergePortMocker != nil {
		ret.handlers = append(ret.handlers, ret.seMergePortMocker)

		svcMocker.SetDispatcher(func(meta resource.Metadata, item *networkingapi.ServiceEntry) {
			ev := source.BuildServiceEntryEvent(event.Updated, item, meta)
			for _, h := range ret.handlers {
				h.Handle(ev)
			}
		})

		ret.initWg.Add(1)
	}

	ret.instanceFilter = generateInstanceFilter(args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll, args.AlwaysUseSourceScopedEpSelectors)
	ret.serviceHostAliases = generateServiceHostAliases(args.ServiceHostAliases)
	ret.seMetaModifierFactory = generateSeMetaModifierFactory(args.ServiceAdditionalMetas)
	ret.reGroupInstances = reGroupInstances(args.InstanceMetaRelabel, args.ServiceNaming)

	for _, op := range options {
		if err := op(ret); err != nil {
			return nil, nil, err
		}
	}

	return ret, ret.cacheJson, nil
}

func (s *Source) cacheShallowCopy() map[string]*networkingapi.ServiceEntry {
	s.mut.RLock()
	defer s.mut.RUnlock()
	ret := make(map[string]*networkingapi.ServiceEntry, len(s.cache))
	for k, v := range s.cache {
		ret[k] = v
	}
	return ret
}

func generateInstanceFilter(
	svcSel map[string][]*bootstrap.EndpointSelector,
	epSel []*bootstrap.EndpointSelector,
	emptySelectorsReturn bool,
	alwaysUseSourceScopedEpSelectors bool,
) func(*instance) bool {
	cfgs := make(map[string]source.HookConfig, len(svcSel))
	for svc, selectors := range svcSel {
		cfgs[svc] = source.ConvertEndpointSelectorToHookConfig(selectors, source.HookConfigWithEmptySelectorsReturn(emptySelectorsReturn))
	}
	cfgs[defaultServiceFilter] = source.ConvertEndpointSelectorToHookConfig(epSel, source.HookConfigWithEmptySelectorsReturn(emptySelectorsReturn))
	hookStore := source.NewHookStore(cfgs)
	return func(i *instance) bool {
		param := source.NewHookParam(source.HookParamWithLabels(i.Metadata))
		filter := hookStore[i.ServiceName]
		if filter == nil {
			filter = hookStore[defaultServiceFilter]
			return filter(param)
		}
		if alwaysUseSourceScopedEpSelectors {
			sourceScopedFilter := hookStore[defaultServiceFilter]
			return sourceScopedFilter(param) && filter(param)
		}
		return filter(param)
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

func (s *Source) markServiceEntryInitDone() {
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
		log.Infof("%s service entry init done, close ch and call initWg.Done", SourceName)
		s.initWg.Done()
		close(ch)
	}
}

func (s *Source) onConfig(args *bootstrap.NacosSourceArgs) {
	var prevArgs *bootstrap.NacosSourceArgs
	prevArgs, s.args = s.args, args

	s.mut.Lock()
	if !reflect.DeepEqual(prevArgs.EndpointSelectors, args.EndpointSelectors) ||
		!reflect.DeepEqual(prevArgs.ServicedEndpointSelectors, args.ServicedEndpointSelectors) {
		newInstSel := generateInstanceFilter(args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll, args.AlwaysUseSourceScopedEpSelectors)
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
		newReGroupInstances := reGroupInstances(args.InstanceMetaRelabel, args.ServiceNaming)
		s.reGroupInstances = newReGroupInstances
	}
	s.mut.Unlock()
}

func (s *Source) getInstanceFilters() func(*instance) bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.instanceFilter
}

func (s *Source) getServiceHostAlias() map[string][]string {
	s.mut.RLock()
	defer s.mut.RUnlock()

	return s.serviceHostAliases
}

func (s *Source) getSeMetaModifierFactory() func(string) func(*resource.Metadata) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.seMetaModifierFactory
}

func (s *Source) cacheJson(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(s.cacheShallowCopy(), "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal nacos se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func buildEvent(kind event.Kind, item *networkingapi.ServiceEntry, service, resourceNs string, metaModifier func(meta *resource.Metadata)) (event.Event, error) {
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
			"registry": "nacos",
		},
		Version:     source.GenVersion(),
		FullName:    resource.FullName{Name: resource.LocalName(service), Namespace: resource.Namespace(ns)},
		Annotations: map[string]string{},
	}
	if metaModifier != nil {
		metaModifier(&meta)
	}
	return source.BuildServiceEntryEvent(kind, se, meta), nil
}

func (s *Source) Dispatch(handler event.Handler) {
	if s.handlers == nil {
		s.handlers = make([]event.Handler, 0, 1)
	}
	s.handlers = append(s.handlers, handler)
}

func (s *Source) Start() {
	if s.initedCallback != nil {
		t0 := time.Now()
		go func() {
			s.initWg.Wait()
			monitoring.RecordReady(SourceName, t0, time.Now())
			s.initedCallback(SourceName)
		}()
	}

	go func() {
		if s.args.Mode == POLLING {
			go s.Polling()
		} else {
			go s.Watching()
		}
		<-s.stop
	}()

	if s.seMergePortMocker != nil {
		go func() {
			<-s.seInitCh

			log.Infof("%s service entry init done, begin to do init se merge port refresh", SourceName)
			s.seMergePortMocker.Refresh()
			s.initWg.Done()

			s.seMergePortMocker.Start(nil)
		}()
	}
}

func (s *Source) Stop() {
	s.stop <- struct{}{}
}

func printEps(eps []*networkingapi.WorkloadEntry) string {
	ips := make([]string, 0)
	for _, ep := range eps {
		ips = append(ips, ep.Address)
	}
	return strings.Join(ips, ",")
}

func reGroupInstances(rl *bootstrap.InstanceMetaRelabel, c *bootstrap.ServiceNameConverter) func(in []*instanceResp) []*instanceResp {
	if rl == nil && c == nil {
		return nil
	}

	var instanceRelabel func(inst *instance) = func(inst *instance) { /*do nothing*/ }
	if rl != nil {
		var relabelFuncs []func(inst *instance)
		for _, relabel := range rl.Items {
			f := func(item *bootstrap.InstanceMetaRelabelItem) func(inst *instance) {
				return func(inst *instance) {
					if len(inst.Metadata) == 0 ||
						item.Key == "" || item.TargetKey == "" {
						return
					}
					v, exist := inst.Metadata[item.Key]
					if !exist {
						return
					} else {
						if nv, exist := item.ValuesMapping[v]; exist {
							v = nv
						}
					}
					if _, exist := inst.Metadata[item.TargetKey]; !exist || item.Overwrite {
						inst.Metadata[item.TargetKey] = v
					}
				}
			}(relabel)
			relabelFuncs = append(relabelFuncs, f)
		}
		instanceRelabel = func(inst *instance) {
			for _, f := range relabelFuncs {
				f(inst)
			}
		}
	}

	var instanceDom func(inst *instance) string = func(inst *instance) string { return inst.ServiceName }
	if c != nil {
		var substrFuncs []func(inst *instance) string
		for _, item := range c.Items {
			var substrF func(inst *instance) string
			switch item.Kind {
			case bootstrap.InstanceBasicInfoKind:
				switch item.Value {
				case bootstrap.InstanceBasicInfoSvc:
					substrF = func(inst *instance) string { return inst.ServiceName }
				case bootstrap.InstanceBasicInfoIP:
					substrF = func(inst *instance) string { return inst.Ip }
				case bootstrap.InstanceBasicInfoPort:
					substrF = func(inst *instance) string { return fmt.Sprintf("%d", inst.Port) }
				}
			case bootstrap.InstanceMetadataKind:
				substrF = func(meta string) func(inst *instance) string {
					return func(inst *instance) string {
						if inst.Metadata == nil {
							return ""
						}
						return inst.Metadata[meta]
					}
				}(item.Value)
			case bootstrap.StaticKind:
				substrF = func(staticValue string) func(inst *instance) string {
					return func(inst *instance) string { return staticValue }
				}(item.Value)
			}
			substrFuncs = append(substrFuncs, substrF)
		}
		instanceDom = func(inst *instance) string {
			subs := make([]string, 0, len(c.Items))
			for _, f := range substrFuncs {
				subs = append(subs, f(inst))
			}
			svcName := strings.Join(subs, c.Sep)
			// overwrite the original service name
			inst.ServiceName = svcName
			return svcName
		}
	}

	return func(in []*instanceResp) []*instanceResp {
		m := map[string][]*instance{}
		for _, ir := range in {
			for _, host := range ir.Hosts {
				instanceRelabel(host)
				dom := instanceDom(host)
				m[dom] = append([]*instance(m[dom]), host)
			}
		}
		out := make([]*instanceResp, 0, len(m))
		for dom, hosts := range m {
			out = append(out, &instanceResp{
				Dom:   dom,
				Hosts: hosts,
			})
		}
		return out
	}
}
