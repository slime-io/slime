package zookeeper

import (
	"time"

	"github.com/go-zookeeper/zk"
	cmap "github.com/orcaman/concurrent-map"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
)

func (s *Source) Polling() {
	go func() {
		ticker := time.NewTicker(time.Duration(s.args.RefreshPeriod))
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
	children, _, err := s.Con.Load().(*zk.Conn).Children(s.args.RegistryRootNode)
	if err != nil {
		log.Errorf("zk path %s get child error: %s", s.args.RegistryRootNode, err.Error())
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
	providerChild, _, err := s.Con.Load().(*zk.Conn).Children(s.args.RegistryRootNode + "/" + service + "/" + ProviderNode)
	if err != nil {
		log.Errorf("zk %s get provider error: %s", service, err.Error())
		return
	}

	var consumerChild []string
	if s.args.GatewayModel {
		consumerChild = make([]string, 0)
	} else {
		consumerChild, _, err = s.Con.Load().(*zk.Conn).Children(s.args.RegistryRootNode + "/" + service + "/" + ConsumerNode)
		if err != nil {
			log.Errorf("zk %s get consumer error: %s", service, err.Error())
		}
	}

	s.handleServiceData(s.pollingCache, providerChild, consumerChild, service)
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
