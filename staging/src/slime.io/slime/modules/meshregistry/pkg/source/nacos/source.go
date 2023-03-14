package nacos

import (
	"encoding/json"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	cmap "github.com/orcaman/concurrent-map"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/pkg/log"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

type Source struct {
	args *bootstrap.NacosSourceArgs // should only be accessed in `onConfig`

	// nacos client
	client            Client
	namingClient      naming_client.INamingClient
	seMergePortMocker *source.ServiceEntryMergePortMocker

	// common configs
	group           string
	patchLabel      bool
	gatewayModel    bool
	nsHost          bool
	k8sDomainSuffix bool
	allNamespaces   bool
	namespace       string
	namespaces      []string
	svcPort         uint32
	mode            string
	delay           time.Duration
	refreshPeriod   time.Duration

	// waching configs
	svcNameWithNs bool

	// polling configs
	defaultSvcNs string
	resourceNs   string

	// source cache
	cache             map[string]*networking.ServiceEntry
	namingServiceList cmap.ConcurrentMap
	handlers          []event.Handler

	mut sync.RWMutex

	// source status
	started     bool
	firstInited bool
	stop        chan struct{}
	seInitCh    chan struct{}
	initWg      sync.WaitGroup

	initedCallback func(string)

	// InstanceFilers, the key of the map is the service name, and the corresponding value
	// is the filters applied to this service. If the service name is empty, it means that
	// all services are applicable to this filter.
	instanceFilters source.SelectHookStore
}

var Scope = log.RegisterScope("nacos", "nacos debugging", 0)

const (
	SourceName       = "nacos"
	HttpPath         = "/nacos"
	POLLING          = "polling"
	WATCHING         = "watching"
	clientHeadersEnv = "NACOS_CLIENT_HEADERS"

	allServiceFilter = ""
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
	svcMocker = source.NewServiceEntryMergePortMocker(args.MockServiceEntryName, args.ResourceNs, args.MockServiceName, map[string]string{
		"registry": "nacos",
	})

	s := &Source{
		args: args,

		namespace:       args.Namespace,
		group:           args.Group,
		delay:           delay,
		refreshPeriod:   time.Duration(args.RefreshPeriod),
		mode:            args.Mode,
		svcNameWithNs:   args.NameWithNs,
		started:         false,
		gatewayModel:    args.GatewayModel,
		patchLabel:      args.LabelPatch,
		svcPort:         args.SvcPort,
		nsHost:          nsHost,
		k8sDomainSuffix: k8sDomainSuffix,
		firstInited:     false,
		defaultSvcNs:    args.DefaultServiceNs,
		resourceNs:      args.ResourceNs,

		initedCallback: readyCallback,

		cache:             make(map[string]*networking.ServiceEntry),
		namingServiceList: cmap.New(),
		stop:              make(chan struct{}),
		seInitCh:          make(chan struct{}),

		seMergePortMocker: svcMocker,
	}
	svcMocker.SetDispatcher(func(meta resource.Metadata, item *networking.ServiceEntry) {
		ev := buildEventFromMeta(event.Updated, item, meta)
		for _, h := range s.handlers {
			h.Handle(ev)
		}
	})

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
		s.client = NewClient(args.Address,
			args.Username,
			args.Password,
			args.Namespace,
			args.Group,
			args.MetaKeyNamespace,
			args.MetaKeyGroup,
			args.AllNamespaces,
			headers)
	} else {
		namingClient, err := newNamingClient(args.Address, args.Namespace, headers)
		if err != nil {
			return nil, nil, Error{
				msg: fmt.Sprintf("init nacos client failed: %s", err.Error()),
			}
		}
		s.namingClient = namingClient
	}

	s.initWg.Add(1)
	if s.seMergePortMocker != nil {
		s.initWg.Add(1)
	}

	s.instanceFilters = generateInstanceFilters(args.ServicedEndpointSelectors, args.EndpointSelectors)

	for _, op := range options {
		if err := op(s); err != nil {
			return nil, nil, err
		}
	}

	return s, s.cacheJson, nil
}

func generateInstanceFilters(
	svcSel map[string][]*metav1.LabelSelector, epSel []*metav1.LabelSelector) source.SelectHookStore {
	ret := source.NewSelectHookStore(svcSel)
	ret[allServiceFilter] = source.NewSelectHook(epSel)
	return ret
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
		log.Infof("service entry init done, close ch and call initWg.Done")
		s.initWg.Done()
		close(ch)
	}
}

func (s *Source) onConfig(args *bootstrap.NacosSourceArgs) {
	var prevArgs *bootstrap.NacosSourceArgs
	prevArgs, s.args = s.args, args

	if !reflect.DeepEqual(prevArgs.EndpointSelectors, args.EndpointSelectors) ||
		!reflect.DeepEqual(prevArgs.ServicedEndpointSelectors, args.ServicedEndpointSelectors) {
		newInstSel := generateInstanceFilters(args.ServicedEndpointSelectors, args.EndpointSelectors)
		s.mut.Lock()
		s.instanceFilters = newInstSel
		s.mut.Unlock()
	}
}

func (s *Source) getInstanceFilters() source.SelectHookStore {
	s.mut.RLock()
	defer s.mut.RUnlock()

	return s.instanceFilters
}

func (s *Source) cacheJson(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(s.cache, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal nacos se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func buildEvent(kind event.Kind, item *networking.ServiceEntry, service, resourceNs string) (event.Event, error) {
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
		Version:     source.GenVersion(collections.K8SNetworkingIstioIoV1Alpha3Serviceentries),
		FullName:    resource.FullName{Name: resource.LocalName(service), Namespace: resource.Namespace(ns)},
		Annotations: map[string]string{},
	}

	return buildEventFromMeta(kind, se, meta), nil
}

func buildEventFromMeta(kind event.Kind, se *networking.ServiceEntry, meta resource.Metadata) event.Event {
	source.FillRevision(meta)
	util.FillSeLabels(se, meta)
	return event.Event{
		Kind:   kind,
		Source: collections.K8SNetworkingIstioIoV1Alpha3Serviceentries,
		Resource: &resource.Instance{
			Metadata: meta,
			Message:  se,
		},
	}
}

func (s *Source) Dispatch(handler event.Handler) {
	if s.handlers == nil {
		s.handlers = make([]event.Handler, 0, 1)
	}
	s.handlers = append(s.handlers, handler)
}

func (s *Source) Start() {
	if s.initedCallback != nil {
		go func() {
			s.initWg.Wait()
			s.initedCallback(SourceName)
		}()
	}

	go func() {
		if s.mode == POLLING {
			go s.Polling()
		} else {
			go s.Watching()
		}
		<-s.stop
	}()

	if s.seMergePortMocker != nil {
		go func() {
			<-s.seInitCh

			log.Infof("service entry init done, begin to do init se merge port refresh")
			s.seMergePortMocker.Refresh()
			s.initWg.Done()

			s.seMergePortMocker.Start(nil)
		}()
	}
}

func (s *Source) Stop() {
	s.stop <- struct{}{}
}

func printEps(eps []*networking.WorkloadEntry) string {
	ips := make([]string, 0)
	for _, ep := range eps {
		ips = append(ips, ep.Address)
	}
	return strings.Join(ips, ",")
}
