package eureka

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "eureka")

type Source struct {
	args *bootstrap.EurekaSourceArgs

	delay time.Duration

	initedCallback func(string)
	handlers       []event.Handler

	stop     chan struct{}
	started  bool
	seInitCh chan struct{}
	initWg   sync.WaitGroup
	mut      sync.RWMutex

	cache map[string]*networking.ServiceEntry

	seMergePortMocker *source.ServiceEntryMergePortMocker
	client            Client
}

const (
	SourceName = "eureka"
	HttpPath   = "/eureka"
)

func New(args *bootstrap.EurekaSourceArgs, delay time.Duration, readyCallback func(string)) (event.Source, func(http.ResponseWriter, *http.Request), error) {
	serviers := args.Servers
	if len(serviers) == 0 {
		serviers = []bootstrap.EurekaServer{args.EurekaServer}
	}
	client := NewClients(serviers)
	if client == nil {
		return nil, nil, Error{
			msg: "Init eureka client failed",
		}
	}

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

	if !args.InstancePortAsSvcPort && args.SvcPort == 0 {
		return nil, nil, fmt.Errorf("SvcPort == 0 while InstancePortAsSvcPort false is not permitted")
	}

	ret := &Source{
		args:    args,
		delay:   delay,
		started: false,

		initedCallback: readyCallback,

		cache: make(map[string]*networking.ServiceEntry),

		stop:     make(chan struct{}),
		seInitCh: make(chan struct{}),

		client:            client,
		seMergePortMocker: svcMocker,
	}

	ret.initWg.Add(1) // service entry init-sync
	if svcMocker != nil {
		ret.handlers = append(ret.handlers, ret.seMergePortMocker)

		svcMocker.SetDispatcher(func(meta resource.Metadata, item *networking.ServiceEntry) {
			ev := source.BuildServiceEntryEvent(event.Updated, item, meta)
			for _, h := range ret.handlers {
				h.Handle(ev)
			}
		})

		ret.initWg.Add(1)
	}

	return ret, ret.cacheJson, nil
}

func (s *Source) cacheShallowCopy() map[string]*networking.ServiceEntry {
	s.mut.RLock()
	defer s.mut.RUnlock()
	ret := make(map[string]*networking.ServiceEntry, len(s.cache))
	for k, v := range s.cache {
		ret[k] = v
	}
	return ret
}

func (s *Source) cacheJson(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(s.cacheShallowCopy(), "", "  ")
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
	t0 := time.Now()
	log.Infof("eureka refresh start : %d", t0.UnixNano())
	if err := s.updateServiceInfo(); err != nil {
		monitoring.RecordPolling(SourceName, t0, time.Now(), false)
		log.Errorf("eureka update service info failed: %v", err)
		return
	}
	t1 := time.Now()
	log.Infof("eureka refresh finsh : %d", t1.UnixNano())
	monitoring.RecordPolling(SourceName, t0, t1, true)

	s.markServiceEntryInitDone()
}

func (s *Source) updateServiceInfo() error {
	apps, err := s.client.Applications()
	if err != nil {
		return fmt.Errorf("get eureka app failed: %v", err)
	}
	newServiceEntryMap, err := ConvertServiceEntryMap(
		apps, s.args.DefaultServiceNs, s.args.GatewayModel, s.args.LabelPatch, s.args.SvcPort,
		s.args.InstancePortAsSvcPort, s.args.NsHost, s.args.K8sDomainSuffix, s.args.NsfEureka)
	if err != nil {
		log.Errorf("convert eureka servceentry map failed: " + err.Error())
		return fmt.Errorf("convert eureka servceentry map failed: %v", err)
	}

	cache := s.cacheShallowCopy()

	for service, se := range cache {
		if _, ok := newServiceEntryMap[service]; !ok {
			// DELETE ==> set ep size to zero
			seCopy := *se
			seCopy.Endpoints = make([]*networking.WorkloadEntry, 0)
			newServiceEntryMap[service] = &seCopy
			se = &seCopy
			event, err := buildEvent(event.Updated, se, service, s.args.ResourceNs)
			if err == nil {
				log.Infof("delete(update) eureka se, hosts: %s ,ep: %s ,size : %d ", se.Hosts[0], printEps(se.Endpoints), len(se.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build delete event for %s failed: %v", service, err)
			}
			monitoring.RecordServiceEntryDeletion(SourceName, false, err == nil)
		}
	}

	for service, newEntry := range newServiceEntryMap {
		if oldEntry, ok := cache[service]; !ok {
			// ADD
			event, err := buildEvent(event.Added, newEntry, service, s.args.ResourceNs)
			if err == nil {
				log.Infof("add eureka se, hosts: %s ,ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build add event for %s failed: %v", service, err)
			}
			monitoring.RecordServiceEntryCreation(SourceName, err == nil)
		} else {
			if !proto.Equal(oldEntry, newEntry) {
				// UPDATE
				event, err := buildEvent(event.Updated, newEntry, service, s.args.ResourceNs)
				if err == nil {
					log.Infof("update eureka se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handlers {
						h.Handle(event)
					}
				} else {
					log.Errorf("build update event for %s failed: %v", service, err)
				}
				monitoring.RecordServiceEntryUpdate(SourceName, err == nil)
			}
		}
	}

	s.mut.Lock()
	s.cache = newServiceEntryMap
	s.mut.Unlock()

	return nil
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
			"registry": SourceName,
		},
		Version:     source.GenVersion(),
		FullName:    resource.FullName{Name: resource.LocalName(service), Namespace: resource.Namespace(ns)},
		Annotations: map[string]string{},
	}

	return source.BuildServiceEntryEvent(kind, se, meta), nil
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

func printEps(eps []*networking.WorkloadEntry) string {
	ips := make([]string, 0)
	for _, ep := range eps {
		ips = append(ips, ep.Address)
	}
	return strings.Join(ips, ",")
}
