package nacos

import (
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	networking "istio.io/api/networking/v1alpha3"
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
			Scope.Error("can not Parse address %s", add)
		}
		port := uint64(80)
		if url.Port() != "" {
			p, err := strconv.ParseInt(url.Port(), 10, 64)
			if err == nil {
				port = uint64(p)
			} else {
				Scope.Errorf("{%s} parse port error %v", add, err)
			}
		}
		serverConfigs = append(serverConfigs, constant.ServerConfig{
			IpAddr:      url.Hostname(),
			ContextPath: "/nacos",
			Port:        port,
			Scheme:      "http",
		})
	}

	namingClient, err := clients.NewNamingClient(
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
		ticker := time.NewTicker(s.refreshPeriod)
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
	if !s.firstInited {
		s.firstInited = true
		s.initedCallback(SourceName)
	}
}

func (s *Source) parseNacosService(serviceInfos model.ServiceList, subscribe bool) {
	for _, serviceName := range serviceInfos.Doms {
		newService := s.namingServiceList.SetIfAbsent(serviceName, true)
		if newService {
			Scope.Infof("parse nacos service %v", serviceInfos.Doms)
			instances, e := s.namingClient.SelectInstances(vo.SelectInstancesParam{
				ServiceName: serviceName,
				GroupName:   s.group,
				HealthyOnly: true,
			})
			if e != nil {
				Scope.Errorf("get %s instances failed", serviceName)
			}
			Scope.Infof("parse nacos instance %v", instances)
			newServiceEntryMap, err := ConvertServiceEntryMapForNacos(serviceName, instances, s.svcNameWithNs, s.gatewayModel, s.svcPort, s.nsHost, s.k8sDomainSuffix, s.patchLabel)
			if err != nil {
				Scope.Errorf("convert nacos servceentry map failed: " + err.Error())
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
	Scope.Infof("nacos start sub %s", serviceName)
	go s.namingClient.Subscribe(&vo.SubscribeParam{
		ServiceName: serviceName,
		GroupName:   s.group,
		SubscribeCallback: func(ins []model.Instance, err error) {
			if err != nil {
				Scope.Infof("nacos sub error  %v", serviceName, err)
				s.deleteService(serviceName)
				return
			}
			Scope.Infof("nacos sub %s changed, %v", serviceName, ins)
			newServiceEntryMap, err := ConvertServiceEntryMapForNacos(serviceName, ins, s.svcNameWithNs, s.gatewayModel, s.svcPort, s.nsHost, s.k8sDomainSuffix, s.patchLabel)
			if err != nil {
				Scope.Errorf("convert nacos servceentry map failed: " + err.Error())
				return
			}
			s.updateService(newServiceEntryMap)
		},
	})
}

func (s *Source) updateNacosService() {
	serviceInfos, err := s.namingClient.GetAllServicesInfo(vo.GetAllServiceInfoParam{
		NameSpace: s.namespace,
		GroupName: s.group,
		PageNo:    1,
		PageSize:  10000,
	})
	if err != nil {
		Scope.Errorf("get all service info error %s", err.Error())
		return
	}
	s.parseNacosService(serviceInfos, true)
	if serviceInfos.Count > 10000 {
		pages := (serviceInfos.Count / 10000) + 1
		for i := int64(2); i <= pages; i++ {
			serviceInfos, err = s.namingClient.GetAllServicesInfo(vo.GetAllServiceInfoParam{
				NameSpace: s.namespace,
				GroupName: s.group,
				PageNo:    uint32(i),
				PageSize:  10000,
			})
			if err != nil {
				Scope.Errorf("get all service info error %s", err.Error())
				return
			}
			s.parseNacosService(serviceInfos, true)
		}
	}
}

func (s *Source) deleteService(serviceName string) {
	for service, oldEntry := range s.cache {
		if service.nacosService == serviceName {
			// DELETE, set ep size to zero
			oldEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
			if event, err := buildEvent(event.Updated, oldEntry, service.Name()); err == nil {
				Scope.Infof("delete(update) nacos se, hosts: %s ,ep: %s ,size : %d ", oldEntry.Hosts[0], printEps(oldEntry.Endpoints), len(oldEntry.Endpoints))
				for _, h := range s.handler {
					h.Handle(event)
				}
			}
		}
	}
}

func (s *Source) updateService(newServiceEntryMap map[serviceEntryNameWapper]*networking.ServiceEntry) {
	for service, newEntry := range newServiceEntryMap {
		if oldEntry, ok := s.cache[service]; !ok {
			// ADD
			s.cache[service] = newEntry
			if event, err := buildEvent(event.Added, newEntry, service.Name()); err == nil {
				Scope.Infof("add nacos se, hosts: %s ,ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
				for _, h := range s.handler {
					h.Handle(event)
				}
			}
		} else {
			if !reflect.DeepEqual(oldEntry, newEntry) {
				// UPDATE
				s.cache[service] = newEntry
				if event, err := buildEvent(event.Updated, newEntry, service.Name()); err == nil {
					Scope.Infof("update nacos se, hosts: %s, ep: %s, size: %d ", newEntry.Hosts[0], printEps(newEntry.Endpoints), len(newEntry.Endpoints))
					for _, h := range s.handler {
						h.Handle(event)
					}
				}
			}
		}
	}
}

// 浦发 servicename 格式： group@@svc@@cluster  or  group@@svc
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
