package nacos

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
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
	// nacos client
	client       Client
	namingClient naming_client.INamingClient
	namespace    string
	group        string

	// common configs
	patchLabel      bool
	gatewayModel    bool
	nsHost          bool
	k8sDomainSuffix bool
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
	handler           []event.Handler

	// source status
	started     bool
	firstInited bool
	stop        chan struct{}

	initedCallback func(string)
}

var (
	instanceFilter                       = new(atomic.Value) // source.SelectHook
	ApplyInstanceFilter source.ApplyHook = func(sh source.SelectHook) {
		instanceFilter.Store(sh)
	}

	Scope = log.RegisterScope("nacos", "nacos debugging", 0)
)

const (
	SourceName       = "nacos"
	HttpPath         = "/nacos"
	POLLING          = "polling"
	WATCHING         = "watching"
	clientHeadersEnv = "NACOS_CLIENT_HEADERS"
)

func New(nacoesArgs bootstrap.NacosSourceArgs, nsHost bool, k8sDomainSuffix bool, delay time.Duration, readyCallback func(string)) (event.Source, func(http.ResponseWriter, *http.Request), error) {
	s := &Source{
		namespace:         nacoesArgs.Namespace,
		group:             nacoesArgs.Group,
		delay:             delay,
		cache:             make(map[string]*networking.ServiceEntry),
		namingServiceList: cmap.New(),
		refreshPeriod:     time.Duration(nacoesArgs.RefreshPeriod),
		mode:              nacoesArgs.Mode,
		svcNameWithNs:     nacoesArgs.NameWithNs,
		started:           false,
		gatewayModel:      nacoesArgs.GatewayModel,
		patchLabel:        nacoesArgs.LabelPatch,
		svcPort:           nacoesArgs.SvcPort,
		nsHost:            nsHost,
		k8sDomainSuffix:   k8sDomainSuffix,
		stop:              make(chan struct{}),
		firstInited:       false,
		initedCallback:    readyCallback,
		defaultSvcNs:      nacoesArgs.DefaultServiceNs,
		resourceNs:        nacoesArgs.ResourceNs,
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
	if nacoesArgs.Mode == POLLING {
		s.client = NewClient(nacoesArgs.Address, nacoesArgs.Username, nacoesArgs.Password, headers)
	} else {
		namingClient, err := newNamingClient(nacoesArgs.Address, nacoesArgs.Namespace, headers)
		if err != nil {
			return nil, nil, Error{
				msg: fmt.Sprintf("init nacos client failed: %s", err.Error()),
			}
		}
		s.namingClient = namingClient
	}
	source.UpdateSelector(nacoesArgs.EndpointSelectors, ApplyInstanceFilter)
	return s, s.cacheJson, nil
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
	source.FillRevision(meta)
	util.FillSeLabels(se, meta)
	return event.Event{
		Kind:   kind,
		Source: collections.K8SNetworkingIstioIoV1Alpha3Serviceentries,
		Resource: &resource.Instance{
			Metadata: meta,
			Message:  se,
		},
	}, nil
}

func (s *Source) Dispatch(handler event.Handler) {
	if s.handler == nil {
		s.handler = make([]event.Handler, 0, 1)
	}
	s.handler = append(s.handler, handler)
}

func (s *Source) Start() {
	go func() {
		if s.mode == POLLING {
			go s.Polling()
		} else {
			go s.Watching()
		}
		<-s.stop
	}()
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
