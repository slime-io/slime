package nacos

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	cmap "github.com/orcaman/concurrent-map"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/util"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/pkg/log"
	"slime.io/slime/modules/meshregistry/pkg/source"
)

type serviceEntryNameWapper struct {
	nacosService string
	ns           string
}

func (s *serviceEntryNameWapper) Name() string {
	if s.ns != "" {
		return s.nacosService + "." + s.ns
	}
	return s.nacosService
}

type Source struct {
	namespace         string
	group             string
	delay             time.Duration
	cache             map[serviceEntryNameWapper]*networking.ServiceEntry
	client            Client
	namingClient      naming_client.INamingClient
	namingServiceList cmap.ConcurrentMap
	handler           []event.Handler
	refreshPeriod     time.Duration
	mode              string
	svcNameWithNs     bool
	started           bool
	gatewayModel      bool
	patchLabel        bool
	svcPort           uint32
	nsHost            bool
	k8sDomainSuffix   bool
	stop              chan struct{}
	firstInited       bool
	initedCallback    func(string)
}

var Scope = log.RegisterScope("nacos", "nacos debugging", 0)

const (
	SourceName       = "nacos"
	HttpPath         = "/nacos"
	POLLING          = "polling"
	WATCHING         = "watching"
	clientHeadersEnv = "NACOS_CLIENT_HEADERS"
)

func New(nacoesArgs bootstrap.NacosSourceArgs, nsHost bool, k8sDomainSuffix bool, delay time.Duration, readyCallback func(string)) (event.Source, func(http.ResponseWriter, *http.Request), error) {
	source := &Source{
		namespace:         nacoesArgs.Namespace,
		group:             nacoesArgs.Group,
		delay:             delay,
		cache:             make(map[serviceEntryNameWapper]*networking.ServiceEntry),
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
		source.client = NewClient(nacoesArgs.Address, headers)
	} else {
		namingClient, err := newNamingClient(nacoesArgs.Address, nacoesArgs.Namespace, headers)
		if err != nil {
			return nil, nil, Error{
				msg: fmt.Sprint("init nacos client failed: %s", err.Error()),
			}
		}
		source.namingClient = namingClient
	}
	return source, source.cacheJson, nil
}

func (s *Source) cacheJson(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(s.cache, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal eureka se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func buildEvent(kind event.Kind, item *networking.ServiceEntry, service string) (event.Event, error) {
	se := util.CopySe(item)
	items := strings.Split(service, ".")
	ns := service
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
