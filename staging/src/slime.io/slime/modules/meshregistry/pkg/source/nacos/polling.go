package nacos

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
)

func (s *Source) Polling() {
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

func (s *Source) refresh() {
	if s.started {
		return
	}
	defer func() {
		s.started = false
	}()
	s.started = true
	log.Infof("nacos refresh start : %d", time.Now().UnixNano())
	if err := s.updateServiceInfo(); err != nil {
		log.Errorf("eureka update service info failed: %v", err)
		return
	}
	log.Infof("nacos refresh finsh : %d", time.Now().UnixNano())
	s.markServiceEntryInitDone()
}

func (s *Source) updateServiceInfo() error {
	instances, err := s.client.Instances()
	if err != nil {
		return fmt.Errorf("get nacos instances failed: %v", err)

	}

	if s.reGroupInstances != nil {
		instances = s.reGroupInstances(instances)
	}

	newServiceEntryMap, err := ConvertServiceEntryMap(
		instances, s.args.DefaultServiceNs, s.args.GatewayModel, s.args.SvcPort, s.args.NsHost, s.args.K8sDomainSuffix,
		s.args.InstancePortAsSvcPort, s.args.LabelPatch, s.args.NsfNacos, s.getInstanceFilters(), s.getServiceHostAlias())
	if err != nil {
		return fmt.Errorf("convert nacos servceentry map failed: %v", err)
	}

	cache := s.cacheShallowCopy()
	seMetaModifierFactory := s.getSeMetaModifierFactory()
	for service, oldEntry := range cache {
		if _, ok := newServiceEntryMap[service]; !ok {
			// DELETE ==> set ep size to zero
			oldEntryCopy := *oldEntry
			oldEntryCopy.Endpoints = make([]*networking.WorkloadEntry, 0)
			newServiceEntryMap[service] = &oldEntryCopy
			if event, err := buildEvent(event.Updated, oldEntry, service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
				log.Infof("delete(update) nacos se, hosts: %s ,ep: %s ,size : %d ", oldEntry.Hosts[0], printEps(oldEntry.Endpoints), len(oldEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build delete event for %s failed: %v", service, err)
			}
		}
	}

	for service, newEntry := range newServiceEntryMap {
		if oldEntry, ok := cache[service]; !ok {
			// ADD
			if event, err := buildEvent(event.Added, newEntry, service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
				log.Infof("add nacos se, hosts: %s ,ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build add event for %s failed: %v", service, err)
			}
		} else {
			if !proto.Equal(oldEntry, newEntry) {
				// UPDATE
				if event, err := buildEvent(event.Updated, newEntry, service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
					log.Infof("update nacos se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handlers {
						h.Handle(event)
					}
				}
			} else {
				log.Errorf("build update event for %s failed: %v", service, err)
			}
		}
	}

	s.mut.Lock()
	s.cache = newServiceEntryMap
	s.mut.Unlock()

	return nil
}
