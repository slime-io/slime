package controllers

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

func (r *ServicefenceReconciler) StartSvcCache(ctx context.Context) {
	clientSet := r.env.K8SClient
	log := log.WithField("function", "newSvcCache")
	// init service watcher
	servicesClient := clientSet.CoreV1().Services("")
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return servicesClient.List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return servicesClient.Watch(ctx, options)
		},
	}
	_, cacheSync, watcher, _ := watchtools.NewIndexerInformerWatcher(lw, &corev1.Service{})
	r.svcSynced = cacheSync.HasSynced
	go func() {
		// wait for svc cache synced
		cache.WaitForCacheSync(ctx.Done(), r.svcSynced)
		log.Infof("Service cacher is running")
		for {
			select {
			case <-ctx.Done():
				log.Infof("context is closed, break process loop")
				return
			case e, ok := <-watcher.ResultChan():
				if !ok {
					log.Warningf("a result chan of service watcher is closed, break process loop")
					return
				}

				service, ok := e.Object.(*corev1.Service)
				if !ok {
					log.Errorf("invalid type of object in service watcher event")
					continue
				}
				ns := service.GetNamespace()
				name := service.GetName()
				eventSvc := ns + "/" + name
				// delete eventSvc from labelSvcCache to ensure final consistency
				r.labelSvcCache.Lock()
				for label, m := range r.labelSvcCache.Data {
					delete(m, eventSvc)
					if len(m) == 0 {
						delete(r.labelSvcCache.Data, label)
					}
				}
				r.labelSvcCache.Unlock()

				// TODO delete eventSvcPort from portProtocolCache

				// delete event
				// delete eventSvc from ns->svc map
				if e.Type == watch.Deleted {
					r.nsSvcCache.Lock()
					delete(r.nsSvcCache.Data[ns], eventSvc)
					r.nsSvcCache.Unlock()
					// labelSvcCache already deleted, skip
					continue
				}

				// add, update event
				// add eventSvc to nsSvcCache
				r.nsSvcCache.Lock()
				if r.nsSvcCache.Data[ns] == nil {
					r.nsSvcCache.Data[ns] = make(map[string]struct{})
				}
				r.nsSvcCache.Data[ns][eventSvc] = struct{}{}
				r.nsSvcCache.Unlock()
				// add eventSvc to labelSvcCache again
				r.labelSvcCache.Lock()
				for k, v := range service.GetLabels() {
					label := LabelItem{
						Name:  k,
						Value: v,
					}
					if r.labelSvcCache.Data[label] == nil {
						r.labelSvcCache.Data[label] = make(map[string]struct{})
					}
					r.labelSvcCache.Data[label][eventSvc] = struct{}{}
				}
				r.labelSvcCache.Unlock()

				// add eventSvc ports to portProtocolCache again
				if ns != r.env.Config.Global.IstioNamespace {
					r.portProtocolCache.Lock()
					for _, port := range service.Spec.Ports {
						p := port.Port
						portProtos := r.portProtocolCache.Data[p]
						if portProtos == nil {
							portProtos = make(map[Protocol]uint)
							r.portProtocolCache.Data[p] = portProtos
						}
						if isHttp(port) {
							portProtos[ProtocolHTTP]++
						}
					}
					r.portProtocolCache.Unlock()
				}
			}
		}
	}()
}

func (r *ServicefenceReconciler) StartAutoPort(ctx context.Context) {
	log := log.WithField("function", "StartAutoPort")
	wormholePort := r.cfg.WormholePort
	needUpdate, successUpdate := false, true
	go func() {
		// wait for svc cache synced
		cache.WaitForCacheSync(ctx.Done(), r.svcSynced)
		log.Infof("Lazyload port auto management is running")
		// polling request
		pollTicker := time.NewTicker(10 * time.Second)
		// init and retry request
		retryCh := time.After(5 * time.Second)
		for {
			select {
			case <-ctx.Done():
				log.Infof("Lazyload port auto management is terminated")
				return
			case <-pollTicker.C:
			case <-retryCh:
				retryCh = nil
			}

			// update wormholePort
			log.Debugf("got timer event for updating wormholePort")

			wormholePort, needUpdate = updateWormholePort(wormholePort, r.portProtocolCache)
			if needUpdate || !successUpdate {
				log.Debugf("need to update resources")
				successUpdate = updateResources(wormholePort, &r.env)
				if !successUpdate {
					log.Infof("retry to update resources")
					retryCh = time.After(1 * time.Second)
				}
			} else {
				log.Debugf("no need to update resources")
			}
		}
	}()
}

func isHttp(port corev1.ServicePort) bool {
	if port.Protocol != "TCP" {
		return false
	}
	p := strings.Split(port.Name, "-")[0]
	return PortProtocol(p) == HTTP
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

func (r *ServicefenceReconciler) StartIpToSvcCache(ctx context.Context) {

	client := r.env.K8SClient
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Endpoints("").Watch(ctx, metav1.ListOptions{})
		},
	}

	_, controller := cache.NewInformer(lw, &corev1.Endpoints{}, 60*time.Second, cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { r.handleEpAdd(ctx, obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { r.handleEpUpdate(ctx, oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { r.handleEpDelete(ctx, obj) },
	})

	log.Infof("run endpoints controller to construct ipToSvcCache and svcToIpsCache")
	go controller.Run(ctx.Done())
}

func (r *ServicefenceReconciler) handleEpAdd(ctx context.Context, obj interface{}) {
	ep, ok := obj.(*corev1.Endpoints)
	if !ok {
		return
	}
	r.addIpWithEp(ep)
}

func (r *ServicefenceReconciler) handleEpUpdate(ctx context.Context, old, obj interface{}) {

	ep, ok := obj.(*corev1.Endpoints)
	if !ok {
		return
	}
	oldEp, ok := old.(*corev1.Endpoints)
	if !ok {
		return
	}

	if reflect.DeepEqual(oldEp.Subsets, ep.Subsets) {
		return
	}

	r.deleteIpFromEp(oldEp)
	r.addIpWithEp(ep)
}

func (r *ServicefenceReconciler) handleEpDelete(ctx context.Context, obj interface{}) {

	ep, ok := obj.(*corev1.Endpoints)
	if !ok {
		return
	}
	r.deleteIpFromEp(ep)
}

func (r *ServicefenceReconciler) addIpWithEp(ep *corev1.Endpoints) {

	svc := ep.GetNamespace() + "/" + ep.GetName()
	ipToSvcCache := r.ipToSvcCache
	svcToIpsCache := r.svcToIpsCache

	var addresses []string
	ipToSvcCache.Lock()
	for _, subset := range ep.Subsets {
		for _, address := range subset.Addresses {
			addresses = append(addresses, address.IP)
			if _, ok := ipToSvcCache.Data[address.IP]; !ok {
				ipToSvcCache.Data[address.IP] = make(map[string]struct{})
			}
			ipToSvcCache.Data[address.IP][svc] = struct{}{}
		}
	}
	ipToSvcCache.Unlock()

	svcToIpsCache.Lock()
	svcToIpsCache.Data[svc] = addresses
	svcToIpsCache.Unlock()
}

func (r *ServicefenceReconciler) deleteIpFromEp(ep *corev1.Endpoints) {

	svc := ep.GetNamespace() + "/" + ep.GetName()
	ipToSvcCache := r.ipToSvcCache
	svcToIpsCache := r.svcToIpsCache

	// delete svc in svcToIpsCache
	svcToIpsCache.Lock()
	ips := svcToIpsCache.Data[svc]
	delete(svcToIpsCache.Data, svc)
	svcToIpsCache.Unlock()

	// delete ips related svc
	ipToSvcCache.Lock()
	for _, ip := range ips {
		delete(ipToSvcCache.Data, ip)
	}
	ipToSvcCache.Unlock()
}
