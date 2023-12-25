package nacos

import (
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	nacosClients "github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
)

func newNamingClient(addresses []string, namespace string, header map[string]string) (iClient naming_client.INamingClient, err error) {
	// TODO support for header

	threadNumEnv := os.Getenv("NACOS_UPDATETHREADNUM")
	updateThreadNum := 0
	if threadNumEnv != "" {
		intVar, err := strconv.Atoi(threadNumEnv)
		if err == nil {
			updateThreadNum = intVar
		}
	}
	clientConfig := constant.NewClientConfig(constant.WithUpdateCacheWhenEmpty(true), constant.WithNamespaceId(namespace), constant.WithUpdateThreadNum(updateThreadNum))
	serverConfigs := make([]constant.ServerConfig, 0)
	for _, add := range addresses {
		url, err := url.Parse(add)
		if err != nil {
			log.Error("can not Parse address %s", add)
		}
		port := uint64(80)
		if url.Port() != "" {
			p, err := strconv.ParseInt(url.Port(), 10, 64)
			if err == nil {
				port = uint64(p)
			} else {
				log.Errorf("{%s} parse port error %v", add, err)
			}
		}
		serverConfigs = append(serverConfigs, constant.ServerConfig{
			IpAddr:      url.Hostname(),
			ContextPath: "/nacos",
			Port:        port,
			Scheme:      "http",
		})
	}

	namingClient, err := nacosClients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	return namingClient, err
}

func (s *Source) Watching() {
	go func() {
		time.Sleep(s.delay)
		ticker := time.NewTicker(time.Duration(s.args.RefreshPeriod))
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.watch()
			}
		}
	}()
}

func (s *Source) watch() {
	if s.started {
		return
	}
	defer func() {
		s.started = false
	}()
	s.started = true
	// Scope.Infof("nacos refresh start : %d", time.Now().UnixNano())
	s.updateNacosService()
	// Scope.Infof("nacos refresh finish : %d", time.Now().UnixNano())
	s.markServiceEntryInitDone()
}

func (s *Source) parseNacosService(serviceInfos model.ServiceList, subscribe bool) {
	for _, serviceName := range serviceInfos.Doms {
		newService := s.namingServiceList.SetIfAbsent(serviceName, true)
		if newService {
			log.Infof("parse nacos service %v", serviceInfos.Doms)
			instances, e := s.namingClient.SelectInstances(vo.SelectInstancesParam{
				ServiceName: serviceName,
				GroupName:   s.args.Group,
				HealthyOnly: true,
			})
			if e != nil {
				log.Errorf("get %s instances failed", serviceName)
			}
			log.Infof("parse nacos instance %v", instances)
			newServiceEntryMap, err := ConvertServiceEntryMapForNacos(serviceName, instances, s.args.NameWithNs, s.args.GatewayModel, s.args.SvcPort, s.args.NsHost, s.args.K8sDomainSuffix, s.args.LabelPatch)
			if err != nil {
				log.Errorf("convert nacos servceentry map failed: " + err.Error())
				return
			}
			s.updateService(newServiceEntryMap)
			if subscribe {
				s.subNacosService(serviceName)
			}
		}
	}
}

func (s *Source) subNacosService(serviceName string) {
	log.Infof("nacos start sub %s", serviceName)
	go s.namingClient.Subscribe(&vo.SubscribeParam{
		ServiceName: serviceName,
		GroupName:   s.args.Group,
		SubscribeCallback: func(ins []model.Instance, err error) {
			if err != nil {
				log.Infof("nacos sub error  %v", serviceName, err)
				s.deleteService(serviceName)
				return
			}
			log.Infof("nacos sub %s changed, %v", serviceName, ins)
			newServiceEntryMap, err := ConvertServiceEntryMapForNacos(serviceName, ins, s.args.NameWithNs, s.args.GatewayModel, s.args.SvcPort, s.args.NsHost, s.args.K8sDomainSuffix, s.args.LabelPatch)
			if err != nil {
				log.Errorf("convert nacos servceentry map failed: " + err.Error())
				return
			}
			s.updateService(newServiceEntryMap)
		},
	})
}

func (s *Source) updateNacosService() {
	serviceInfos, err := s.namingClient.GetAllServicesInfo(vo.GetAllServiceInfoParam{
		NameSpace: s.args.Namespace,
		GroupName: s.args.Group,
		PageNo:    1,
		PageSize:  10000,
	})
	if err != nil {
		log.Errorf("get all service info error %s", err.Error())
		return
	}
	s.parseNacosService(serviceInfos, true)
	if serviceInfos.Count > 10000 {
		pages := (serviceInfos.Count / 10000) + 1
		for i := int64(2); i <= pages; i++ {
			serviceInfos, err = s.namingClient.GetAllServicesInfo(vo.GetAllServiceInfoParam{
				NameSpace: s.args.Namespace,
				GroupName: s.args.Group,
				PageNo:    uint32(i),
				PageSize:  10000,
			})
			if err != nil {
				log.Errorf("get all service info error %s", err.Error())
				return
			}
			s.parseNacosService(serviceInfos, true)
		}
	}
}

func (s *Source) deleteService(serviceName string) {
	s.mut.Lock()
	se := s.cache[serviceName]
	if se != nil {
		// DELETE ==> set ep size to zero
		seCopy := *se
		seCopy.Endpoints = make([]*networkingapi.WorkloadEntry, 0)
		s.cache[serviceName] = &seCopy
		se = &seCopy
	}
	s.mut.Unlock()

	if se != nil {
		if event, err := buildEvent(event.Updated, se, serviceName, s.args.ResourceNs, nil); err == nil {
			log.Infof("delete(update) nacos se, hosts: %s ,ep: %s ,size : %d ", se.Hosts[0], printEps(se.Endpoints), len(se.Endpoints))
			for _, h := range s.handlers {
				h.Handle(event)
			}
		} else {
			log.Errorf("build delete event for %s failed: %v", serviceName, err)
		}
	}
}

func (s *Source) updateService(newServiceEntryMap map[string]*networkingapi.ServiceEntry) {
	type update struct {
		service            string
		oldEntry, newEntry *networkingapi.ServiceEntry
	}

	var updates []update
	s.mut.Lock()
	for service, newEntry := range newServiceEntryMap {
		updates = append(updates, update{
			service:  service,
			oldEntry: s.cache[service],
			newEntry: newEntry,
		})
		s.cache[service] = newEntry
	}
	s.mut.Unlock()

	for _, up := range updates {
		newEntry := up.newEntry
		if up.oldEntry == nil {
			// ADD
			if event, err := buildEvent(event.Added, newEntry, up.service, s.args.ResourceNs, nil); err == nil {
				log.Infof("add nacos se, hosts: %s ,ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("build add event for %s failed: %v", up.service, err)
			}
		} else {
			if !reflect.DeepEqual(up.oldEntry, newEntry) {
				// UPDATE
				if event, err := buildEvent(event.Updated, newEntry, up.service, s.args.ResourceNs, nil); err == nil {
					log.Infof("update nacos se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handlers {
						h.Handle(event)
					}
				} else {
					log.Errorf("build update event for %s failed: %v", up.service, err)
				}
			}
		}
	}
}

// servicename 格式： group@@svc@@cluster  or  group@@svc
func getServiceName(originName string) string {
	items := strings.SplitN(originName, "@@", 3)
	if len(items) > 1 {
		return items[1]
	} else {
		return originName
	}
}

func getStringEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
	}
	return value
}

func getIntEnv(key string, fallback int64) int64 {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	intValue, error := strconv.ParseInt(value, 10, 64)
	if error == nil {
		return intValue
	} else {
		return fallback
	}
}

func getBooleanEnv(key string, fallback bool) bool {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	boolValue, error := strconv.ParseBool(value)
	if error == nil {
		return boolValue
	} else {
		return fallback
	}
}
