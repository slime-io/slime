package zookeeper

import (
	"fmt"
	"time"

	"slime.io/slime/framework/util"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
)

func (s *Source) Polling() {
	go func() {
		ticker := time.NewTicker(time.Duration(s.args.RefreshPeriod))
		defer ticker.Stop()
		for {
			s.refresh()

			forceUpdateTrigger := s.forceUpdateTrigger.Load().(chan struct{})
			select {
			case <-s.stop:
				return
			case <-ticker.C:
			case <-forceUpdateTrigger:
			}
		}
	}()
}

func (s *Source) refresh() {
	t0 := time.Now()
	log.Infof("zk refresh start : %d", t0.UnixNano())
	if err := s.updateServiceInfo(); err != nil {
		monitoring.RecordPolling(SourceName, t0, time.Now(), false)
		log.Errorf("nacos update service info failed: %v", err)
		return
	}
	t1 := time.Now()
	log.Infof("zk refresh finish : %d", t1.UnixNano())
	monitoring.RecordPolling(SourceName, t0, t1, true)
	s.markServiceEntryInitDone()
}

func (s *Source) updateServiceInfo() error {
	interfaces, err := s.Con.Children(s.args.RegistryRootNode)
	monitoring.RecordSourceClientRequest(SourceName, err == nil)
	if err != nil {
		return fmt.Errorf("zk path %s get child error: %s", s.args.RegistryRootNode, err.Error())
	}
	for _, iface := range interfaces {
		s.iface(iface)
	}
	s.handleInterfacesDelete(interfaces)
	return nil
}

func (s *Source) iface(service string) {
	providers, err := s.Con.Children(s.args.RegistryRootNode + "/" + service + "/" + ProviderNode)
	monitoring.RecordSourceClientRequest(SourceName, err == nil)
	if err != nil {
		log.Errorf("zk %s get provider error: %s", service, err.Error())
		return
	}

	var consumers []string
	if consumerPath := s.args.ConsumerPath; consumerPath == "" {
		consumers = make([]string, 0)
	} else {
		consumers, err = s.Con.Children(s.args.RegistryRootNode + "/" + service + consumerPath)
		monitoring.RecordSourceClientRequest(SourceName, err == nil)
		if err != nil {
			log.Debugf("zk %s get consumer error: %s", service, err.Error())
		}
	}

	var configurators []string
	if s.args.EnableConfiguratorMeta {
		configurators, err = s.Con.Children(s.args.RegistryRootNode + "/" + service + "/" + ConfiguratorNode)
		if err != nil {
			log.Debugf("zk %s get configurator error: %s", service, err.Error())
		}
	}

	s.handleServiceData(providers, consumers, configurators, service)
}

func (s *Source) handleInterfacesDelete(currInterfaces []string) {
	existMap := make(map[string]string)
	for _, iface := range currInterfaces {
		existMap[iface] = iface
	}
	deleteInterfaces := make([]string, 0)
	for service := range s.cache.Items() {
		if _, exist := existMap[service]; !exist {
			deleteInterfaces = append(deleteInterfaces, service)
		}
	}

	for _, iface := range deleteInterfaces {
		s.handleServiceDelete(iface, util.NewSet[string]())
	}
}
