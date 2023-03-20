package zookeeper

import (
	"reflect"
	"strconv"
	"time"

	"github.com/go-zookeeper/zk"
	cmap "github.com/orcaman/concurrent-map"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
)

func (s *Source) Polling() {
	go func() {
		ticker := time.NewTicker(s.refreshPeriod)
		defer ticker.Stop()
		for {
			s.refresh()

			select {
			case <-s.stop:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Source) refresh() {
	log.Infof("zk refresh start : %d", time.Now().UnixNano())
	children, _, err := s.Con.Load().(*zk.Conn).Children(s.RegisterRootNode)
	if err != nil {
		log.Errorf("zk path %s get child error: %s", s.RegisterRootNode, err.Error())
		return
	}
	for _, child := range children {
		s.iface(child)
	}
	s.handleNodeDelete(children)
	log.Infof("zk refresh finish : %d", time.Now().UnixNano())
	s.markServiceEntryInitDone()
}

func (s *Source) iface(service string) {
	providerChild, _, err := s.Con.Load().(*zk.Conn).Children(s.RegisterRootNode + "/" + service + "/" + ProviderNode)
	if err != nil {
		log.Errorf("zk %s get provider error: %s", service, err.Error())
		return
	}

	var consumerChild []string
	if s.zkGatewayModel {
		consumerChild = make([]string, 0)
	} else {
		consumerChild, _, err = s.Con.Load().(*zk.Conn).Children(s.RegisterRootNode + "/" + service + "/" + ConsumerNode)
		if err != nil {
			log.Errorf("zk %s get consumer error: %s", service, err.Error())
		}
	}

	s.handleServiceData(s.pollingCache, providerChild, consumerChild, service)
}

func (s *Source) handleServiceData(cacheInUse cmap.ConcurrentMap, provider, consumer []string, service string) {
	if _, ok := cacheInUse.Get(service); !ok {
		cacheInUse.Set(service, cmap.New())
	}

	freshSeMap := convertServiceEntry(provider, consumer, service, s.patchLabel, s.ignoreLabels, s.zkGatewayModel)
	for serviceKey, convertedSe := range freshSeMap {
		se := convertedSe.se
		now := time.Now()
		newSeWithMeta := &ServiceEntryWithMeta{
			ServiceEntry: se,
			Meta: resource.Metadata{
				FullName:   resource.FullName{Namespace: resource.Namespace(s.resourceNs), Name: resource.LocalName(serviceKey)},
				CreateTime: now,
				Version:    resource.Version(now.String()),
				Labels: map[string]string{
					"path":            service,
					"registry":        "zookeeper",
					dubboParamMethods: convertedSe.methodsLabel,
				},
				Annotations: map[string]string{},
			},
		}

		if !convertedSe.methodsEqual {
			newSeWithMeta.Meta.Labels[DubboSvcMethodEqLabel] = strconv.FormatBool(convertedSe.methodsEqual)
		}

		v, ok := cacheInUse.Get(service)
		if !ok {
			continue
		}
		seCache, ok := v.(cmap.ConcurrentMap)
		if !ok {
			continue
		}

		callModel := convertDubboCallModel(se, convertedSe.InboundEndPoints)

		if value, exist := seCache.Get(serviceKey); !exist {
			seCache.Set(serviceKey, newSeWithMeta)
			if ev, err := buildSeEvent(event.Added, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, callModel); err == nil {
				log.Infof("add zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
		} else if existSeWithMeta, ok := value.(*ServiceEntryWithMeta); ok {
			if reflect.DeepEqual(existSeWithMeta, newSeWithMeta) {
				continue
			}
			seCache.Set(serviceKey, newSeWithMeta)
			if ev, err := buildSeEvent(event.Updated, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, callModel); err == nil {
				log.Infof("update zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
		}
	}

	// check if svc deleted
	deleteKey := make([]string, 0)
	v, ok := cacheInUse.Get(service)
	if !ok {
		return
	}
	seCache, ok := v.(cmap.ConcurrentMap)
	if !ok {
		return
	}
	for serviceKey, v := range seCache.Items() {
		_, exist := freshSeMap[serviceKey]
		if exist {
			continue
		}
		deleteKey = append(deleteKey, serviceKey)
		seValue, ok := v.(*ServiceEntryWithMeta)
		if !ok {
			continue
		}

		// del event -> empty-ep update event
		seValue.ServiceEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
		ev, err := buildSeEvent(event.Updated, seValue.ServiceEntry, seValue.Meta, nil)
		if err != nil {
			log.Errorf("delete svc failed, case: %v", err.Error())
			continue
		}
		log.Infof("delete(update) zk se, hosts: %s, ep size: %d ", seValue.ServiceEntry.Hosts[0], len(seValue.ServiceEntry.Endpoints))
		for _, h := range s.handlers {
			h.Handle(ev)
		}
	}

	for _, key := range deleteKey {
		seCache.Remove(key)
	}
}

func (s *Source) handleNodeDelete(childrens []string) {
	existMap := make(map[string]string)
	for _, child := range childrens {
		existMap[child] = child
	}
	deleteKey := make([]string, 0)
	for service := range s.pollingCache.Items() {
		if _, exist := existMap[service]; !exist {
			deleteKey = append(deleteKey, service)
		}
	}
	for _, service := range deleteKey {
		if seCache, ok := s.pollingCache.Get(service); ok {
			if ses, castok := seCache.(cmap.ConcurrentMap); castok {
				for _, v := range ses.Items() {
					if seValue, ok := v.(*ServiceEntryWithMeta); ok {
						seValue.ServiceEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
						if event, err := buildSeEvent(event.Updated, seValue.ServiceEntry, seValue.Meta, nil); err == nil {
							log.Infof("delete(update) zk se, hosts: %s, ep size: %d ", seValue.ServiceEntry.Hosts[0], len(seValue.ServiceEntry.Endpoints))
							for _, h := range s.handlers {
								h.Handle(event)
							}
						} else {
							log.Errorf("delete(update) svc failed, case: %v", err.Error())
						}
					}
				}
			}
		}
	}
}
