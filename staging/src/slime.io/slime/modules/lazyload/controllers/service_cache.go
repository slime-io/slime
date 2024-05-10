package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"slime.io/slime/modules/lazyload/model"
)

func (r *ServicefenceReconciler) StartCache(ctx context.Context) {
	factory := informers.NewSharedInformerFactory(r.env.K8SClient, 0)
	r.factory = factory

	svcInformer := factory.Core().V1().Services().Informer()
	epInformer := factory.Core().V1().Endpoints().Informer()
	_ = factory.Core().V1().Namespaces().Informer()

	svcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r.handleSvcAdd(ctx, obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.handleSvcUpdate(ctx, oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			r.handleSvcDelete(ctx, obj)
		},
	})

	epInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r.handleEpAdd(ctx, obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.handleEpUpdate(ctx, oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			r.handleEpDelete(ctx, obj)
		},
	})
	go factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	log.Infof("factory has synced in startCache")
}

func (r *ServicefenceReconciler) handleSvcAdd(_ context.Context, obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}

	r.addLabelSvcCache(svc)
	r.addNsSvcCache(svc)
	r.addPortProtocolCache(svc)
}

func (r *ServicefenceReconciler) handleSvcUpdate(_ context.Context, old, obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}

	oldSvc, ok := old.(*corev1.Service)
	if !ok {
		return
	}

	if reflect.DeepEqual(svc.Spec, oldSvc.Spec) {
		return
	}

	r.deleteLabelSvcCache(oldSvc)
	r.addLabelSvcCache(svc)

	r.deleteNsSvcCache(oldSvc)
	r.addNsSvcCache(svc)

	r.deletePortProtocolCache(oldSvc)
	r.addPortProtocolCache(svc)
}

func (r *ServicefenceReconciler) handleSvcDelete(_ context.Context, obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}

	r.deleteLabelSvcCache(svc)

	r.deleteNsSvcCache(svc)

	r.deletePortProtocolCache(svc)
}

func (r *ServicefenceReconciler) addLabelSvcCache(svc *corev1.Service) {
	ns := svc.GetNamespace()
	nn := fmt.Sprintf("%s/%s", ns, svc.GetName())

	r.labelSvcCache.Lock()
	defer r.labelSvcCache.Unlock()
	for k, v := range svc.GetLabels() {
		label := LabelItem{Name: k, Value: v}

		if r.labelSvcCache.Data[label] == nil {
			r.labelSvcCache.Data[label] = make(map[string]struct{})
		}
		r.labelSvcCache.Data[label][nn] = struct{}{}
	}
}

func (r *ServicefenceReconciler) deleteLabelSvcCache(svc *corev1.Service) {
	ns := svc.GetNamespace()
	nn := fmt.Sprintf("%s/%s", ns, svc.GetName())
	r.labelSvcCache.Lock()
	defer r.labelSvcCache.Unlock()
	for label, m := range r.labelSvcCache.Data {
		delete(m, nn)
		if len(m) == 0 {
			delete(r.labelSvcCache.Data, label)
		}
	}
}

func (r *ServicefenceReconciler) addNsSvcCache(svc *corev1.Service) {
	ns := svc.GetNamespace()
	nn := fmt.Sprintf("%s/%s", ns, svc.GetName())

	r.nsSvcCache.Lock()
	defer r.nsSvcCache.Unlock()
	if r.nsSvcCache.Data[ns] == nil {
		r.nsSvcCache.Data[ns] = make(map[string]struct{})
	}
	r.nsSvcCache.Data[ns][nn] = struct{}{}
}

func (r *ServicefenceReconciler) deleteNsSvcCache(svc *corev1.Service) {
	ns := svc.GetNamespace()
	nn := fmt.Sprintf("%s/%s", ns, svc.GetName())

	r.nsSvcCache.Lock()
	defer r.nsSvcCache.Unlock()
	delete(r.nsSvcCache.Data[ns], nn)
}

func (r *ServicefenceReconciler) addPortProtocolCache(svc *corev1.Service) {
	// portProtocolCache have all ports of all services except global-sidecar
	if svc.Name == model.GlobalSidecar {
		return
	}

	r.portProtocolCache.Lock()
	for _, port := range svc.Spec.Ports {
		if !isHttp(port, r.cfg.SupportH2) {
			continue
		}
		p := port.Port
		portProtos := r.portProtocolCache.Data[p]
		if portProtos == nil {
			portProtos = make(map[Protocol]int32)
			r.portProtocolCache.Data[p] = portProtos
		}
		portProtos[ListenerProtocolHTTP]++
	}
	r.portProtocolCache.Unlock()

	log.Debugf("protocol cache: %+v", r.portProtocolCache.Data)
}

func (r *ServicefenceReconciler) deletePortProtocolCache(svc *corev1.Service) {
	// portProtocolCache have all ports of all services except global-sidecar
	if svc.Name == model.GlobalSidecar {
		return
	}

	if !r.cfg.GetCleanupWormholePort() {
		return
	}

	r.portProtocolCache.Lock()
	for _, port := range svc.Spec.Ports {
		p := port.Port
		if !isHttp(port, r.cfg.SupportH2) {
			continue
		}
		if _, exist := r.portProtocolCache.Data[p]; exist {
			r.portProtocolCache.Data[p][ListenerProtocolHTTP]--
		}
	}
	r.portProtocolCache.Unlock()

	log.Debugf("protocol cache: %+v", r.portProtocolCache.Data)
}

func (r *ServicefenceReconciler) StartAutoPort(ctx context.Context) {
	log := log.WithField("function", "StartAutoPort")
	initPort := r.cfg.WormholePort
	needUpdate, successUpdate := false, true

	wormholePort := make([]string, 0)
	sets := make(map[string]struct{})
	for _, port := range initPort {
		sets[port] = struct{}{}
	}

	go func() {
		// wait for svc cache synced
		cache.WaitForCacheSync(ctx.Done(), r.factory.Core().V1().Services().Informer().HasSynced)
		log.Infof("Lazyload port auto management is running, init gs wormholePort: %v", initPort)

		// list all svc and get http port
		svcs, err := r.factory.Core().V1().Services().Lister().List(labels.NewSelector())
		if err == nil {
			for _, svc := range svcs {
				for _, port := range svc.Spec.Ports {
					if !isHttp(port, r.cfg.SupportH2) {
						continue
					}
					sets[strconv.Itoa(int(port.Port))] = struct{}{}
				}
			}
		} else {
			// if list all svc failed, use initPort
			log.Errorf("list all svc failed in autoport: %v", err)
		}

		for port := range sets {
			wormholePort = append(wormholePort, port)
		}
		sort.Strings(wormholePort)
		log.Infof("all wormholeport from initport and informer : %v", wormholePort)
		firstUpdate := true
		// polling request
		pollTicker := time.NewTicker(10 * time.Second)
		// init and retry request
		retryCh := time.After(5 * time.Second)

		for {
			// update wormholePort at first time or needUpdate or update failed
			wormholePort, needUpdate = reloadWormholePort(wormholePort, r.portProtocolCache, r.cfg.GetCleanupWormholePort())
			// hits firstUpdate at first time
			if firstUpdate || needUpdate || !successUpdate {
				if firstUpdate {
					log.Infof("first time to update resources")
					firstUpdate = false
				}
				log.Debugf("need to update resources")
				successUpdate = updateResources(wormholePort, &r.env)
				if !successUpdate {
					UpdateExtraResourceFailed.Increment()
					log.Infof("retry to update resources")
					retryCh = time.After(1 * time.Second)
				}
			} else {
				log.Debugf("no need to update resources")
			}

			select {
			case <-ctx.Done():
				log.Infof("Lazyload port auto management is terminated")
				return
			case <-pollTicker.C:
			case <-retryCh:
				retryCh = nil
			}
		}
	}()
}

// shielding differences, uniformly use http
func isHttp(port corev1.ServicePort, supportH2 bool) bool {
	if port.Protocol != "TCP" {
		return false
	}
	p := strings.Split(port.Name, "-")[0]
	protocol := PortProtocol(p)

	filter := []PortProtocol{HTTP}
	// grpc-web-xx is also split into grpc
	if supportH2 {
		filter = append(filter, GRPC, HTTP2)
	}

	for _, f := range filter {
		if protocol == f {
			return true
		}
	}
	return false
}

func reloadWormholePort(
	wormholePort []string,
	portProtocolCache *PortProtocolCache,
	cleaupWormholePort bool,
) ([]string, bool) {
	updated := false
	ports := make([]string, 0)

	wormholePortMap := make(map[string]bool)
	for _, p := range wormholePort {
		wormholePortMap[p] = true
	}

	cachePorts := make([]string, 0)
	cachePortMap := make(map[string]bool)

	portProtocolCache.RLock()
	for port, proto := range portProtocolCache.Data {
		p := strconv.Itoa(int(port))
		if proto[ListenerProtocolHTTP] > 0 {
			cachePorts = append(cachePorts, p)
			cachePortMap[p] = true
		}
	}
	portProtocolCache.RUnlock()

	// not to clean up wormhole port, merge cache ports and wormhole ports
	if !cleaupWormholePort {
		// add cache ports that are not in the wormholePort
		ports = append(ports, wormholePort...)
		for p := range cachePortMap {
			if !wormholePortMap[p] {
				ports = append(ports, p)
			}
		}
	} else {
		// port in wormholePort maybe cleanup
		// and only add cache ports
		ports = append(ports, cachePorts...)
	}

	sort.Strings(ports)
	sort.Strings(wormholePort)

	if !StringSlicesEqual(ports, wormholePort) {
		updated = true
		wormholePort = ports
	}

	return wormholePort, updated
}

func (r *ServicefenceReconciler) handleEpAdd(_ context.Context, obj interface{}) {
	ep, ok := obj.(*corev1.Endpoints)
	if !ok {
		return
	}

	r.addIpWithEp(ep)
}

func (r *ServicefenceReconciler) handleEpUpdate(_ context.Context, old, obj interface{}) {
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

func (r *ServicefenceReconciler) handleEpDelete(_ context.Context, obj interface{}) {
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
			r.addIpToSvcCache(svc, address.IP)
		}
		for _, address := range subset.NotReadyAddresses {
			addresses = append(addresses, address.IP)
			r.addIpToSvcCache(svc, address.IP)
		}
	}
	ipToSvcCache.Unlock()

	svcToIpsCache.Lock()
	svcToIpsCache.Data[svc] = addresses
	svcToIpsCache.Unlock()
}

// addIpToCache is unsafe
func (r *ServicefenceReconciler) addIpToSvcCache(svc string, ip string) {
	if _, ok := r.ipToSvcCache.Data[ip]; !ok {
		r.ipToSvcCache.Data[ip] = make(map[string]struct{})
	}
	r.ipToSvcCache.Data[ip][svc] = struct{}{}
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
		// ip maybe related to different svc
		if _, ok := ipToSvcCache.Data[ip]; ok {
			delete(ipToSvcCache.Data[ip], svc)
		}
	}
	ipToSvcCache.Unlock()
}

func (r *ServicefenceReconciler) isNamespaceManaged(ns string) bool {
	obj, exists, err := r.factory.Core().V1().Namespaces().Informer().GetIndexer().GetByKey(ns)
	if err != nil {
		log.Errorf("get namespace %s from cache failed: %v", ns, err)
		return false
	}
	if !exists {
		log.Errorf("namespace %s does not exist in cache", ns)
		return false
	}

	return r.inScope(ns, obj.(*corev1.Namespace))
}

func StringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
