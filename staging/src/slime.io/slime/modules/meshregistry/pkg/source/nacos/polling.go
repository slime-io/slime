package nacos

import (
	"reflect"
	"time"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
)

func (s *Source) Polling() {
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

func (s *Source) refresh() {
	if s.started {
		return
	}
	defer func() {
		s.started = false
	}()
	s.started = true
	Scope.Infof("nacos refresh start : %d", time.Now().UnixNano())
	s.updateServiceInfo()
	Scope.Infof("nacos refresh finsh : %d", time.Now().UnixNano())
	if !s.firstInited {
		s.firstInited = true
		s.initedCallback(SourceName)
	}
}

func (s *Source) updateServiceInfo() {
	instances, err := s.client.Instances()
	if err != nil {
		Scope.Errorf("get nacos instances failed: " + err.Error())
		return
	}
	newServiceEntryMap, err := ConvertServiceEntryMap(instances, s.defaultSvcNs, s.gatewayModel, s.svcPort, s.nsHost, s.k8sDomainSuffix, s.patchLabel, s.getInstanceFilters())
	if err != nil {
		Scope.Errorf("convert nacos servceentry map failed: " + err.Error())
		return
	}
	for service, oldEntry := range s.cache {
		if _, ok := newServiceEntryMap[service]; !ok {
			// DELETE, set ep size to zero
			delete(s.cache, service)
			oldEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
			if event, err := buildEvent(event.Updated, oldEntry, service, s.resourceNs); err == nil {
				Scope.Infof("delete(update) nacos se, hosts: %s ,ep: %s ,size : %d ", oldEntry.Hosts[0], printEps(oldEntry.Endpoints), len(oldEntry.Endpoints))
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
				Scope.Infof("add nacos se, hosts: %s ,ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handler {
					h.Handle(event)
				}
			}
		} else {
			if !reflect.DeepEqual(oldEntry, newEntry) {
				// UPDATE
				s.cache[service] = newEntry
				if event, err := buildEvent(event.Updated, newEntry, service, s.resourceNs); err == nil {
					Scope.Infof("update nacos se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handler {
						h.Handle(event)
					}
				}
			}
		}
	}
}
