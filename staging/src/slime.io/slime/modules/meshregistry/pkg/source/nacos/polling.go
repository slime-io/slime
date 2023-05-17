package nacos

import (
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
	s.updateServiceInfo()
	log.Infof("nacos refresh finsh : %d", time.Now().UnixNano())
	s.markServiceEntryInitDone()
}

func (s *Source) updateServiceInfo() {
	instances, err := s.client.Instances()
	if err != nil {
		log.Errorf("get nacos instances failed: " + err.Error())
		return
	}
	if s.reGroupInstances != nil {
		instances = s.reGroupInstances(instances)
	}
	newServiceEntryMap, err := ConvertServiceEntryMap(
		instances, s.args.DefaultServiceNs, s.args.GatewayModel, s.args.SvcPort, s.args.NsHost, s.args.K8sDomainSuffix,
		s.args.InstancePortAsSvcPort, s.args.LabelPatch, s.getInstanceFilters(), s.getServiceHostAlias())
	if err != nil {
		log.Errorf("convert nacos servceentry map failed: " + err.Error())
		return
	}
	seMetaModifierFactory := s.getSeMetaModifierFactory()
	for service, oldEntry := range s.cache {
		if _, ok := newServiceEntryMap[service]; !ok {
			// DELETE, set ep size to zero
			delete(s.cache, service)
			oldEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
			if event, err := buildEvent(event.Updated, oldEntry, service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
				log.Infof("delete(update) nacos se, hosts: %s ,ep: %s ,size : %d ", oldEntry.Hosts[0], printEps(oldEntry.Endpoints), len(oldEntry.Endpoints))
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
			if event, err := buildEvent(event.Added, newEntry, service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
				log.Infof("add nacos se, hosts: %s ,ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			}
		} else {
			if !proto.Equal(oldEntry, newEntry) {
				// UPDATE
				s.cache[service] = newEntry
				if event, err := buildEvent(event.Updated, newEntry, service, s.args.ResourceNs, seMetaModifierFactory(service)); err == nil {
					log.Infof("update nacos se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handlers {
						h.Handle(event)
					}
				}
			}
		}
	}
}
