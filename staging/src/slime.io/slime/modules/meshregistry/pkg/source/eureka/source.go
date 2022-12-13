package eureka

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"

	"slime.io/slime/modules/meshregistry/pkg/util"

	"istio.io/libistio/pkg/config/schema/collections"
	"slime.io/slime/modules/meshregistry/pkg/source"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/pkg/log"
)

type Source struct {
	cache   map[string]*networking.ServiceEntry
	client  Client
	handler []event.Handler

	// worker        *util.Worker
	delay         time.Duration
	refreshPeriod time.Duration

	svcPort uint32

	defaultSvcNs string
	resourceNs   string

	gatewayModel    bool
	patchLabel      bool
	nsHost          bool
	k8sDomainSuffix bool
	nsfEureka       bool

	stop           chan struct{}
	started        bool
	firstInited    bool
	initedCallback func(string)
}

var Scope = log.RegisterScope("eureka", "eureka debugging", 0)

const (
	SourceName = "eureka"
	HttpPath   = "/eureka"
)

func New(eurekaArgs bootstrap.EurekaSourceArgs, delay time.Duration, readyCallback func(string)) (event.Source, func(http.ResponseWriter, *http.Request), error) {
	client := NewClient(eurekaArgs.Address)
	if client == nil {
		return nil, nil, Error{
			msg: "Init eureka client failed",
		}
	}

	ret := &Source{
		delay:           delay,
		cache:           make(map[string]*networking.ServiceEntry),
		client:          client,
		refreshPeriod:   time.Duration(eurekaArgs.RefreshPeriod),
		started:         false,
		gatewayModel:    eurekaArgs.GatewayModel,
		patchLabel:      eurekaArgs.LabelPatch,
		svcPort:         eurekaArgs.SvcPort,
		nsHost:          eurekaArgs.NsHost,
		k8sDomainSuffix: eurekaArgs.K8sDomainSuffix,
		defaultSvcNs:    eurekaArgs.DefaultServiceNs,
		resourceNs:      eurekaArgs.ResourceNs,
		stop:            make(chan struct{}),
		firstInited:     false,
		initedCallback:  readyCallback,
		nsfEureka:       eurekaArgs.NsfEureka,
	}
	return ret, ret.cacheJson, nil
}

func (s *Source) cacheJson(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(s.cache, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal eureka se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func (s *Source) refresh() {
	if s.started {
		return
	}
	defer func() {
		s.started = false
	}()
	s.started = true

	apps, err := s.client.Applications()
	if err != nil {
		log.Errorf("get eureka app failed: " + err.Error())
		return
	}
	newServiceEntryMap, err := ConvertServiceEntryMap(apps, s.defaultSvcNs, s.gatewayModel, s.patchLabel, s.svcPort, s.nsHost, s.k8sDomainSuffix, s.nsfEureka)
	if err != nil {
		log.Errorf("convert eureka servceentry map failed: " + err.Error())
		return
	}

	for service, oldEntry := range s.cache {
		if _, ok := newServiceEntryMap[service]; !ok {
			// DELETE, set ep size to zero
			delete(s.cache, service)
			oldEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
			if event, err := buildEvent(event.Updated, oldEntry, service, s.resourceNs); err == nil {
				log.Infof("delete(update) eureka se, hosts: %s ,ep: %s ,size : %d ", oldEntry.Hosts[0], printEps(oldEntry.Endpoints), len(oldEntry.Endpoints))
				for _, h := range s.handler {
					h.Handle(event)
				}
			}
		}
	}

	for service, newEntry := range newServiceEntryMap {
		if oldEntry, ok := s.cache[service]; !ok {
			// ADD
			s.cache[service] = newEntry
			if event, err := buildEvent(event.Added, newEntry, service, s.resourceNs); err == nil {
				log.Infof("add eureka se, hosts: %s ,ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handler {
					h.Handle(event)
				}
			}
		} else {
			if !reflect.DeepEqual(oldEntry, newEntry) {
				// UPDATE
				s.cache[service] = newEntry
				if event, err := buildEvent(event.Updated, newEntry, service, s.resourceNs); err == nil {
					log.Infof("update eureka se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handler {
						h.Handle(event)
					}
				}
			}
		}
	}
	if !s.firstInited {
		s.firstInited = true
		s.initedCallback(SourceName)
	}
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
			"registry": "eureka",
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
		time.Sleep(s.delay)
		ticker := time.NewTicker(s.refreshPeriod)
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
