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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func (r *ServicefenceReconciler) StartSvcCache(ctx context.Context) {
	log := log.WithField("function", "svcCache")
	client := r.env.K8SClient

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Services("").Watch(ctx, metav1.ListOptions{})
		},
	}

	_, controller := cache.NewInformer(lw, &corev1.Service{}, 60*time.Second, cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { r.handleSvcAdd(ctx, obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { r.handleSvcUpdate(ctx, oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { r.handleSvcDelete(ctx, obj) },
	})

	r.svcSynced = controller.HasSynced
	log.Infof("run svc controller")
	go controller.Run(ctx.Done())
}

func (r *ServicefenceReconciler) handleSvcAdd(ctx context.Context, obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}

	if r.isNamespaceManaged(svc.GetNamespace()) {
		return
	}

	r.addLabelSvcCache(svc)
	r.addNsSvcCache(svc)
	r.addPortProtocolCache(svc)
}

func (r *ServicefenceReconciler) handleSvcUpdate(ctx context.Context, old, obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	if r.isNamespaceManaged(svc.GetNamespace()) {
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

func (r *ServicefenceReconciler) handleSvcDelete(ctx context.Context, obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}

	if r.isNamespaceManaged(svc.GetNamespace()) {
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

	if r.isNamespaceManaged(svc.GetNamespace()) {
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

	if !r.cfg.GetCleanupWormholePort() {
		return
	}
	if r.isNamespaceManaged(svc.GetNamespace()) {
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

			wormholePort, needUpdate = reloadWormholePort(wormholePort, r.portProtocolCache)
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

func reloadWormholePort(wormholePort []string, portProtocolCache *PortProtocolCache) ([]string, bool) {
	updated := false
	sort.Strings(wormholePort)
	log.Infof("old wormPort is: %v", wormholePort)

	oldPortList := wormholePort
	oldPortMap := make(map[string]bool)
	for _, p := range oldPortList {
		oldPortMap[p] = true
	}

	curPortList := make([]string, 0)
	curPortMap := make(map[string]bool)

	portProtocolCache.RLock()
	defer portProtocolCache.RUnlock()

	for port, proto := range portProtocolCache.Data {
		p := strconv.Itoa(int(port))
		if proto[ListenerProtocolHTTP] > 0 {
			curPortList = append(curPortList, p)
			curPortMap[p] = true
			if !oldPortMap[p] {
				// port is not in oldPortList, added
				updated = true
			}
		}
	}
	// curPortList is constructed, need to check deleted
	if !updated {
		for port := range oldPortMap {
			if !curPortMap[port] {
				// port is not in curPortList, deleted
				updated = true
				break
			}
		}
	}

	return curPortList, updated
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
