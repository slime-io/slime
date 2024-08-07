package nacos

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"

	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
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
	t0 := time.Now()
	log.Infof("nacos refresh start : %d", t0.UnixNano())
	if err := s.updateServiceInfo(); err != nil {
		monitoring.RecordPolling(SourceName, t0, time.Now(), false)
		log.Errorf("nacos update service info failed: %v", err)
		return
	}
	t1 := time.Now()
	log.Infof("nacos refresh finsh : %d", t1.UnixNano())
	monitoring.RecordPolling(SourceName, t0, t1, true)
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

	opts := &convertOptions{
		patchLabel:            s.args.LabelPatch,
		enableProjectCode:     s.args.EnableProjectCode,
		nsHost:                s.args.NsHost,
		k8sDomainSuffix:       s.args.K8sDomainSuffix,
		instancePortAsSvcPort: s.args.InstancePortAsSvcPort,
		svcPort:               s.args.SvcPort,
		defaultSvcNs:          s.args.DefaultServiceNs,
		domSuffix:             s.args.DomSuffix,
		filter:                s.getInstanceFilters(),
		hostAliases:           s.getServiceHostAlias(),
	}
	opts.protocol, opts.protocolName = source.ProtocolName(s.args.SvcProtocol, s.args.GenericProtocol)
	newServiceEntryMap, err := ConvertServiceEntryMap(instances, opts)
	if err != nil {
		return fmt.Errorf("convert nacos servceentry map failed: %v", err)
	}

	cache := s.cacheShallowCopy()
	seMetaModifierFactory := s.getSeMetaModifierFactory()
	for fullName, se := range cache {
		if _, ok := newServiceEntryMap[fullName]; !ok {
			// DELETE ==> set ep size to zero
			seCopy := *se
			seCopy.Endpoints = make([]*networkingapi.WorkloadEntry, 0)
			newServiceEntryMap[fullName] = &seCopy
			se = &seCopy
			event, err := buildEvent(event.Updated, se, fullName, s.args.ResourceNs, seMetaModifierFactory(fullName), s.args.NsHost) //nolint: lll
			if err != nil {
				log.Errorf("build delete event for %s failed: %v", fullName, err)
			} else {
				log.Infof("delete(update) nacos se, hosts: %s ,ep: %s ,size : %d ",
					se.Hosts[0], printEps(se.Endpoints), len(se.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			}
			monitoring.RecordServiceEntryDeletion(SourceName, false, err == nil)
		}
	}

	for fullName, newEntry := range newServiceEntryMap {
		if oldEntry, ok := cache[fullName]; !ok {
			// ADD
			event, err := buildEvent(event.Added, newEntry, fullName, s.args.ResourceNs, seMetaModifierFactory(fullName), s.args.NsHost) //nolint: lll
			if err != nil {
				log.Errorf("build add event for %s failed: %v", fullName, err)
			} else {
				log.Infof("add nacos se, hosts: %s ,ep: %s, size: %d ",
					newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			}
			monitoring.RecordServiceEntryCreation(SourceName, err == nil)
		} else if !proto.Equal(oldEntry, newEntry) {
			// UPDATE
			event, err := buildEvent(event.Updated, newEntry, fullName, s.args.ResourceNs, seMetaModifierFactory(fullName), s.args.NsHost) //nolint: lll
			if err != nil {
				log.Errorf("build update event for %s failed: %v", fullName, err)
			} else {
				log.Infof("update nacos se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0],
					printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			}
			monitoring.RecordServiceEntryUpdate(SourceName, err == nil)
		}
	}

	s.mut.Lock()
	s.cache = newServiceEntryMap
	s.mut.Unlock()

	return nil
}
