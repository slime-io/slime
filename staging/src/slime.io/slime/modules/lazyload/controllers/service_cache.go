package controllers

import (
	"context"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"slime.io/slime/framework/util"
)

func (r *ServicefenceReconciler) startSvcCache() {
	clientSet := r.env.K8SClient
	wormholePort := r.cfg.WormholePort

	log := log.WithField("function", "newSvcCache")
	nsSvcCache := &NsSvcCache{Data: map[string]map[string]struct{}{}}
	labelSvcCache := &LabelSvcCache{Data: map[LabelItem]map[string]struct{}{}}
	portProtocolCache := &PortProtocolCache{Data: map[int32]map[Protocol]uint{}}

	// init service watcher
	servicesClient := clientSet.CoreV1().Services("")
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return servicesClient.List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return servicesClient.Watch(options)
		},
	}
	watcher := util.ListWatcher(context.Background(), lw)

	go func() {
		log.Infof("Service cacher is running")
		for {
			e, ok := <-watcher.ResultChan()
			if !ok {
				log.Warningf("a result chan of service watcher is closed, break process loop")
				return
			}

			service, ok := e.Object.(*v1.Service)
			if !ok {
				log.Errorf("invalid type of object in service watcher event")
				continue
			}
			ns := service.GetNamespace()
			name := service.GetName()
			eventSvc := ns + "/" + name
			// delete eventSvc from labelSvcCache to ensure final consistency
			labelSvcCache.Lock()
			for label, m := range labelSvcCache.Data {
				delete(m, eventSvc)
				if len(m) == 0 {
					delete(labelSvcCache.Data, label)
				}
			}
			labelSvcCache.Unlock()

			// TODO delete eventSvcPort from portProtocolCache

			// delete event
			// delete eventSvc from ns->svc map
			if e.Type == watch.Deleted {
				nsSvcCache.Lock()
				delete(nsSvcCache.Data[ns], eventSvc)
				nsSvcCache.Unlock()
				// labelSvcCache already deleted, skip
				continue
			}

			// add, update event
			// add eventSvc to nsSvcCache
			nsSvcCache.Lock()
			if nsSvcCache.Data[ns] == nil {
				nsSvcCache.Data[ns] = make(map[string]struct{})
			}
			nsSvcCache.Data[ns][eventSvc] = struct{}{}
			nsSvcCache.Unlock()
			// add eventSvc to labelSvcCache again
			labelSvcCache.Lock()
			for k, v := range service.GetLabels() {
				label := LabelItem{
					Name:  k,
					Value: v,
				}
				if labelSvcCache.Data[label] == nil {
					labelSvcCache.Data[label] = make(map[string]struct{})
				}
				labelSvcCache.Data[label][eventSvc] = struct{}{}
			}
			labelSvcCache.Unlock()

			// add eventSvc ports to portProtocolCache again
			if ns != r.env.Config.Global.IstioNamespace {
				portProtocolCache.Lock()
				for _, port := range service.Spec.Ports {
					p := port.Port
					portProtos := portProtocolCache.Data[p]
					if portProtos == nil {
						portProtos = make(map[Protocol]uint)
						portProtocolCache.Data[p] = portProtos
					}
					proto := getProtocol(port)
					portProtos[proto]++
				}
				portProtocolCache.Unlock()
			}

		}
	}()

	needUpdate, successUpdate := false, true

	if r.cfg.AutoPort {
		log.Infof("Lazyload port auto management is running")
		go func() {
			// polling request
			pollTicker := time.NewTicker(10 * time.Second)
			// init and retry request
			retryCh := time.After(1*time.Second)
			for {
				select {
				case <-pollTicker.C:
				case <-retryCh:
					retryCh = nil
				}

				// update wormholePort
				log.Debugf("got timer event for updating wormholePort")

				wormholePort, needUpdate = updateWormholePort(wormholePort, portProtocolCache)
				if needUpdate || !successUpdate {
					log.Debugf("need to update resources")
					successUpdate = updateResources(wormholePort, r.env)
					if !successUpdate {
						log.Infof("retry to update resources")
						retryCh = time.After(1*time.Second)
					}
				} else {
					log.Debugf("no need to update resources")
				}
			}
		}()
	}

	r.nsSvcCache = nsSvcCache
	r.labelSvcCache = labelSvcCache
	r.portProtocolCache = portProtocolCache

	return
}

// find protocol of service port
func getProtocol(port v1.ServicePort) Protocol {
	if port.Protocol != "TCP" {
		return ProtocolUnknown
	}
	p := strings.Split(port.Name, "-")[0]
	return portProtocolToProtocol(PortProtocol(p))
}

func portProtocolToProtocol(p PortProtocol) Protocol {
	switch p {
	case HTTP, HTTP2, GRPC, GRPCWeb:
		return ProtocolHTTP
	case TCP, HTTPS, TLS, Mongo, Redis, MySQL:
		return ProtocolTCP
	default:
		return ProtocolUnknown
	}
}

func updateWormholePort(wormholePort []string, portProtocolCache *PortProtocolCache) ([]string, bool) {
	portProtocolCache.RLock()
	defer portProtocolCache.RUnlock()

	var add []string
	wormPortMap := make(map[string]bool)

	for _, p := range wormholePort {
		wormPortMap[p] = true
	}

	for port, proto := range portProtocolCache.Data {
		p := strconv.Itoa(int(port))
		if proto[ProtocolHTTP] > 0 && !wormPortMap[p] {
			add = append(add, p)
		}
	}

	// todo delete wormholePort in future

	wormholePort = append(wormholePort, add...)
	return wormholePort, len(add) > 0
}
