package eureka

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
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

const (
	SourceName = "eureka"

	HttpPath = "/eureka"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "eureka")

func init() {
	source.RegisterSourceInitlizer(SourceName, source.RegistrySourceInitlizer(New))
}

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

	cache map[string]*networkingapi.ServiceEntry

	seMergePortMocker *source.ServiceEntryMergePortMocker
	client            Client
}

func New(
	moduleArgs *bootstrap.RegistryArgs,
	readyCallback func(string),
	_ func(func(*bootstrap.RegistryArgs)),
) (event.Source, map[string]http.HandlerFunc, bool, bool, error) {
	args := moduleArgs.EurekaSource
	if !args.Enabled {
		return nil, nil, false, true, nil
	}

	var svcMocker *source.ServiceEntryMergePortMocker
	if args.MockServiceEntryName != "" {
		svcMocker = source.NewServiceEntryMergePortMocker(
			args.MockServiceEntryName, args.ResourceNs, args.MockServiceName,
			args.MockServiceMergeInstancePort, args.MockServiceMergeServicePort,
			map[string]string{
				"registry": SourceName,
			})
	}

	src := &Source{
		args:              args,
		delay:             time.Duration(moduleArgs.RegistryStartDelay),
		started:           false,
		initedCallback:    readyCallback,
		cache:             make(map[string]*networkingapi.ServiceEntry),
		stop:              make(chan struct{}),
		seInitCh:          make(chan struct{}),
		seMergePortMocker: svcMocker,
	}

	serviers := args.Servers
	if len(serviers) == 0 {
		serviers = []bootstrap.EurekaServer{args.EurekaServer}
	}
	src.client = NewClients(serviers)

	src.initWg.Add(1) // service entry init-sync
	if src.seMergePortMocker != nil {
		src.handlers = append(src.handlers, src.seMergePortMocker)
		src.seMergePortMocker.SetDispatcher(func(meta resource.Metadata, item *networkingapi.ServiceEntry) {
			ev := source.BuildServiceEntryEvent(event.Updated, item, meta)
			for _, h := range src.handlers {
				h.Handle(ev)
			}
		})
		src.initWg.Add(1)
	}

	debugHandler := map[string]http.HandlerFunc{
		HttpPath: src.handleHttp,
	}

	return src, debugHandler, args.LabelPatch, false, nil
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

func (s *Source) handleHttp(w http.ResponseWriter, req *http.Request) {
	queries := req.URL.Query()
	if queries.Get(source.CacheRegistryInfoQueryKey) == "true" {
		s.dumpClients(w, req)
		return
	}
	// default cacheJson
	s.cacheJson(w, req)
}

func (s *Source) dumpClients(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(s.client.RegistryInfo()))
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

	opts := &convertOptions{
		patchLabel:            s.args.LabelPatch,
		enableProjectCode:     s.args.EnableProjectCode,
		nsHost:                s.args.NsHost,
		k8sDomainSuffix:       s.args.K8sDomainSuffix,
		instancePortAsSvcPort: s.args.InstancePortAsSvcPort,
		svcPort:               s.args.SvcPort,
		defaultSvcNs:          s.args.DefaultServiceNs,
		appSuffix:             s.args.AppSuffix,
	}
	opts.protocol, opts.protocolName = source.ProtocolName(s.args.SvcProtocol, s.args.GenericProtocol)
	newServiceEntryMap, err := ConvertServiceEntryMap(apps, opts)
	if err != nil {
		log.Errorf("convert eureka servceentry map failed: " + err.Error())
		return fmt.Errorf("convert eureka servceentry map failed: %v", err)
	}

	cache := s.cacheShallowCopy()
	for seFullName, se := range cache {
		if _, ok := newServiceEntryMap[seFullName]; !ok {
			// DELETE ==> set ep size to zero
			seCopy := *se
			seCopy.Endpoints = make([]*networkingapi.WorkloadEntry, 0)
			newServiceEntryMap[seFullName] = &seCopy
			se = &seCopy
			event, err := buildEvent(event.Updated, se, seFullName, s.args.ResourceNs, s.args.NsHost)
			if err == nil {
				log.Infof("delete(update) eureka se, hosts: %s ,ep: %s ,size : %d ",
					se.Hosts[0], printEps(se.Endpoints), len(se.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build delete event for %s failed: %v", seFullName, err)
			}
			monitoring.RecordServiceEntryDeletion(SourceName, false, err == nil)
		}
	}

	for seFullName, newEntry := range newServiceEntryMap {
		if oldEntry, ok := cache[seFullName]; !ok {
			// ADD
			event, err := buildEvent(event.Added, newEntry, seFullName, s.args.ResourceNs, s.args.NsHost)
			if err == nil {
				log.Infof("add eureka se, hosts: %s ,ep: %s, size: %d ",
					newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build add event for %s failed: %v", seFullName, err)
			}
			monitoring.RecordServiceEntryCreation(SourceName, err == nil)
		} else if !proto.Equal(oldEntry, newEntry) {
			// UPDATE
			event, err := buildEvent(event.Updated, newEntry, seFullName, s.args.ResourceNs, s.args.NsHost)
			if err == nil {
				log.Infof("update eureka se, hosts: %s, ep: %s, size: %d ",
					newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build update event for %s failed: %v", seFullName, err)
			}
			monitoring.RecordServiceEntryUpdate(SourceName, err == nil)
		}
	}

	s.mut.Lock()
	s.cache = newServiceEntryMap
	s.mut.Unlock()

	return nil
}

func buildEvent(
	kind event.Kind,
	item *networkingapi.ServiceEntry,
	seFullName string,
	resourceNs string,
	nsHost bool,
) (event.Event, error) {
	se := util.CopySe(item)
	ns := resourceNs
	if nsHost {
		// pick the last one as Namespace if the NsHost is enabled.
		items := strings.Split(seFullName, ".")
		if len(items) > 1 {
			ns = items[len(items)-1]
		}
	}
	now := time.Now()
	meta := resource.Metadata{
		CreateTime: now,
		Labels: map[string]string{
			"registry": SourceName,
		},
		Version:     source.GenVersion(),
		FullName:    resource.FullName{Name: resource.LocalName(seFullName), Namespace: resource.Namespace(ns)},
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

		// If wait time is set, we will call the initedCallback after wait time.
		if s.args.WaitTime > 0 {
			go func() {
				time.Sleep(time.Duration(s.args.WaitTime))
				s.initedCallback(SourceName)
			}()
		}
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

func printEps(eps []*networkingapi.WorkloadEntry) string {
	ips := make([]string, 0)
	for _, ep := range eps {
		ips = append(ips, ep.Address)
	}
	return strings.Join(ips, ",")
}
