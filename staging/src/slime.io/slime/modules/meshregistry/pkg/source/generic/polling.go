package generic

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
)

func (s *Source[I, APP]) polling() {
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

func (s *Source[I, APP]) refresh() {
	if s.started {
		return
	}
	defer func() {
		s.started = false
	}()
	s.started = true
	log.Infof("%s refresh start", s.registry)
	if err := s.updateServiceInfo(); err != nil {
		log.Errorf("%s update service info failed:%v", s.registry, err)
		return
	}
	log.Infof("%s refresh finsh", s.registry)
	s.markServiceEntryInitDone()
}

func (s *Source[I, APP]) updateServiceInfo() error {
	instances, err := s.client.Applications()
	if err != nil {
		return fmt.Errorf("%s get instances failed: %v", s.registry, err)
	}
	if s.reGroupInstances != nil {
		instances = s.reGroupInstances(instances)
	}

	newServiceEntryMap, err := convertServiceEntryMap(instances, s.registry, s.args.DefaultServiceNs, s.args.SvcPort,
		s.args.GatewayModel, s.args.NSFRegistry, s.nsHost, s.k8sDomainSuffix, s.args.LabelPatch, s.args.InstancePortAsSvcPort,
		s.getInstanceFilters(), s.getServiceHostAlias())
	if err != nil {
		return fmt.Errorf("%s convert servceentry map failed: %s", s.registry, err.Error())
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
	return nil
}
