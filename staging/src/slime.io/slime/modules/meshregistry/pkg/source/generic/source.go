package generic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

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

type Application[I Instance[I], T any] interface {
	GetProjectCodes() []string
	GetInstances() []I
	GetDomain() string
	New(string, []I) T
}

type Client[I Instance[I], IR Application[I, IR]] interface {
	// Applications registered on the registery
	Applications() ([]IR, error)
	ServerInfo() string
}

type Option[I Instance[I], APP Application[I, APP]] func(s *Source[I, APP]) error

func WithDynamicConfigOption[I Instance[I], APP Application[I, APP]](addCb func(func(*bootstrap.SourceArgs))) Option[I, APP] {
	return func(s *Source[I, APP]) error {
		addCb(s.onConfig)
		return nil
	}
}

type Source[I Instance[I], APP Application[I, APP]] struct {
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
	client Client[I, APP]

	// xds
	cache             map[string]*networking.ServiceEntry
	seMergePortMocker *source.ServiceEntryMergePortMocker
	handlers          []event.Handler

	// dynamic config
	mut                   sync.RWMutex
	instanceFilter        func(I) bool
	reGroupInstances      func(in []APP) []APP
	serviceHostAliases    map[string][]string
	seMetaModifierFactory func(string) func(*resource.Metadata)
}

func NewSource[I Instance[I], APP Application[I, APP]](
	args *bootstrap.SourceArgs,
	registry string,
	nsHost, k8sDomainSuffix bool,
	delay time.Duration,
	readyCallback func(string),
	client Client[I, APP],
	options ...Option[I, APP],
) (*Source[I, APP], error) {
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
	ret := &Source[I, APP]{
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
	ret.reGroupInstances = reGroupInstances[I, APP](args.InstanceMetaRelabel, args.ServiceNaming)

	for _, op := range options {
		if err := op(ret); err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (s *Source[I, APP]) onConfig(args *bootstrap.SourceArgs) {
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
		newReGroupInstances := reGroupInstances[I, APP](args.InstanceMetaRelabel, args.ServiceNaming)
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

func (s *Source[I, APP]) Start() {
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

func (s *Source[I, APP]) Stop() {
	s.stop <- struct{}{}
}

func (s *Source[I, APP]) Dispatch(handler event.Handler) {
	if s.handlers == nil {
		s.handlers = make([]event.Handler, 0, 1)
	}
	s.handlers = append(s.handlers, handler)
}

func (s *Source[I, APP]) markServiceEntryInitDone() {
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

func (s *Source[I, APP]) getInstanceFilters() func(I) bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.instanceFilter
}

func (s *Source[I, APP]) getServiceHostAlias() map[string][]string {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.serviceHostAliases
}

func (s *Source[I, APP]) getSeMetaModifierFactory() func(string) func(*resource.Metadata) {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.seMetaModifierFactory
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

func reGroupInstances[I Instance[I], APP Application[I, APP]](rl *bootstrap.InstanceMetaRelabel,
	c *bootstrap.ServiceNameConverter) func(in []APP) []APP {
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

	return func(in []APP) []APP {
		m := map[string][]I{}
		for _, ir := range in {
			for _, inst := range ir.GetInstances() {
				instanceRelabel(inst)
				dom := instanceDom(inst)
				m[dom] = append([]I(m[dom]), inst)
			}
		}
		out := make([]APP, 0, len(m))
		for dom, insts := range m {
			ir := new(APP)
			out = append(out, (*ir).New(dom, insts))
		}
		return out
	}
}
