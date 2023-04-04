package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"slime.io/slime/framework/model"
	"slime.io/slime/framework/model/metric"
	lazyloadv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
)

const (
	LabelServiceFenced = "slime.io/serviceFenced"
	ServiceFencedTrue  = "true"
	ServiceFencedFalse = "false"

	LabelCreatedBy           = "app.kubernetes.io/created-by"
	CreatedByFenceController = "fence-controller"
)

func (r *ServicefenceReconciler) WatchMetric(ctx context.Context) {
	log := log.WithField("reporter", "ServicefenceReconciler").WithField("function", "WatchMetric")
	log.Infof("start watching metric")

	for {
		select {
		case <-ctx.Done():
			log.Infof("context is done, break process loop")
			return
		case metric, ok := <-r.watcherMetricChan:
			if !ok {
				log.Warningf("watcher mertic channel closed, break process loop")
				return
			}
			r.ConsumeMetric(metric)
		case metric, ok := <-r.tickerMetricChan:
			if !ok {
				log.Warningf("ticker metric channel closed, break process loop")
				return
			}
			r.ConsumeMetric(metric)
		}
	}
}

func (r *ServicefenceReconciler) ConsumeMetric(metric metric.Metric) {
	for meta, results := range metric {
		log.Debugf("got metric for %s", meta)
		namespace, name := strings.Split(meta, "/")[0], strings.Split(meta, "/")[1]
		nn := types.NamespacedName{Namespace: namespace, Name: name}
		if len(results) != 1 {
			log.Errorf("wrong metric results length for %s", meta)
			continue
		}
		value := results[0].Value
		if _, err := r.Refresh(reconcile.Request{NamespacedName: nn}, value); err != nil {
			log.Errorf("refresh error:%v", err)
		}
	}
}

func (r *ServicefenceReconciler) Refresh(req reconcile.Request, value map[string]string) (reconcile.Result, error) {
	log := log.WithField("reporter", "ServicefenceReconciler").WithField("function", "Refresh")

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	sf := &lazyloadv1alpha1.ServiceFence{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, sf)
	if err != nil {
		if errors.IsNotFound(err) {
			sf = nil
			err = nil
		} else {
			log.Errorf("can not get ServiceFence %s, %+v", req.NamespacedName.Name, err)
			return reconcile.Result{}, err
		}
	}

	if sf == nil {
		log.Info("ServiceFence Not Found, skip")
		return reconcile.Result{}, nil
	} else if rev := model.IstioRevFromLabel(sf.Labels); !r.env.RevInScope(rev) {
		log.Infof("existing sf %v istioRev %s but our %s, skip ...",
			req.NamespacedName, rev, r.env.IstioRev())
		return reconcile.Result{}, nil
	}

	log.Debugf("refresh with servicefence %s metricstatus old: %v, new: %v", req.NamespacedName, sf.Status.MetricStatus, value)
	// skip refresh when metric result has not changed
	if mapStrStrEqual(sf.Status.MetricStatus, value) {
		return reconcile.Result{}, nil
	}
	// use updateVisitedHostStatus to update svf.spec and svf.status
	sf.Status.MetricStatus = value
	r.updateServicefenceDomain(sf)

	if sf.Spec.Enable {
		if err := r.refreshSidecar(sf); err != nil {
			// XXX return err?
			log.Errorf("refresh sidecar %v met err: %v", req.NamespacedName, err)
		}
	}

	return reconcile.Result{}, nil
}

func mapStrStrEqual(m1, m2 map[string]string) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v1 := range m1 {
		v2, exist := m2[k]
		if !exist {
			return false
		}
		if v2 != v1 {
			return false
		}
	}
	return true
}

// second bool value means: is it clearly known to be managed
func (r *ServicefenceReconciler) isNsFenced(ns *corev1.Namespace) (bool, bool) {
	if ns != nil && ns.Labels != nil {
		switch ns.Labels[LabelServiceFenced] {
		case ServiceFencedTrue:
			return true, true
		case ServiceFencedFalse:
			return false, true
		}
	}
	return false, false
}

func (r *ServicefenceReconciler) isServiceFenced(ctx context.Context, svc *corev1.Service) (bool, error) {
	var svcLabel string
	var err error

	if svc.Labels != nil {
		svcLabel = svc.Labels[LabelServiceFenced]
	}

	switch svcLabel {
	case ServiceFencedFalse:
		return false, nil
	case ServiceFencedTrue:
		return true, nil
	default:
		// refer to ns and default value
		ns := &corev1.Namespace{}
		if err = r.Client.Get(ctx, types.NamespacedName{
			Namespace: "",
			Name:      svc.Namespace,
		}, ns); err != nil {
			if errors.IsNotFound(err) {
				log.Errorf("namespace %s is not found in isServiceFenced", svc.Namespace)
			} else {
				log.Errorf("fail to get ns: %s", svc.Namespace)
			}
			return false, err
		}

		if ns != nil {
			if fenced, ok := r.isNsFenced(ns); ok {
				return fenced, nil
			}
		}
		return r.cfg.DefaultFence, nil
	}
}

func (r *ServicefenceReconciler) ReconcileService(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.inSpecialNs(req.Namespace) {
		//log.Debugf("auto fence does not apply to specifical namespace: %s, skip", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	return r.refreshFenceStatusOfService(ctx, nil, req.NamespacedName)
}

func (r *ServicefenceReconciler) ReconcileNamespace(ctx context.Context, req ctrl.Request) (ret ctrl.Result, err error) {

	if r.inSpecialNs(req.Name) {
		//log.Debugf("auto fence does not apply to specifical namespace: %s, skip", req.NamespacedName)
		return reconcile.Result{}, nil
	}

	// Fetch the namespace instance
	ns := &corev1.Namespace{}
	err = r.Client.Get(ctx, req.NamespacedName, ns)
	if err != nil {
		if errors.IsNotFound(err) {
			ns = nil
			return reconcile.Result{}, nil // do not process deletion ...
		} else {
			log.Errorf("get namespace %s error, %+v", req.NamespacedName, err)
			return reconcile.Result{}, err
		}
	}

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	// refresh service fenced status
	services := &corev1.ServiceList{}
	if err = r.Client.List(ctx, services, client.InNamespace(req.Name)); err != nil {
		log.Errorf("list services %s failed, %+v", req.Name, err)
		return reconcile.Result{}, err
	}

	for _, svc := range services.Items {
		if ret, err = r.refreshFenceStatusOfService(ctx, &svc, types.NamespacedName{}); err != nil {
			log.Errorf("refreshFenceStatusOfService services %s failed, %+v", svc.Name, err)
			return ret, err
		}
	}

	return ctrl.Result{}, nil
}

// refreshFenceStatusOfService caller should hold the reconcile lock.
func (r *ServicefenceReconciler) refreshFenceStatusOfService(ctx context.Context, svc *corev1.Service, nsName types.NamespacedName) (reconcile.Result, error) {
	if svc == nil {
		// Fetch the Service instance
		svc = &corev1.Service{}
		err := r.Client.Get(ctx, nsName, svc)
		if err != nil {
			if errors.IsNotFound(err) {
				svc = nil
			} else {
				log.Errorf("get service %s error, %+v", nsName, err)
				return reconcile.Result{}, err
			}
		}
	} else {
		nsName = types.NamespacedName{
			Namespace: svc.Namespace,
			Name:      svc.Name,
		}
	}

	// Fetch the ServiceFence instance
	sf := &lazyloadv1alpha1.ServiceFence{}
	err := r.Client.Get(ctx, nsName, sf)
	if err != nil {
		if errors.IsNotFound(err) {
			sf = nil
		} else {
			log.Errorf("get serviceFence %s error, %+v", nsName, err)
			return reconcile.Result{}, err
		}
	}

	if sf == nil {
		// ignore services without label selector
		if svc != nil && &(svc.Spec) != nil && svc.Spec.Selector != nil &&
			len(svc.Spec.Selector) > 0 {

			if fenced, err := r.isServiceFenced(ctx, svc); err != nil {
				return reconcile.Result{}, err
			} else if fenced {
				// add svc -> add sf
				sf = &lazyloadv1alpha1.ServiceFence{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Name:      svc.Name,
						Namespace: svc.Namespace,
					},
					Spec: lazyloadv1alpha1.ServiceFenceSpec{
						Enable: true,
						WorkloadSelector: &lazyloadv1alpha1.WorkloadSelector{
							FromService: true,
						},
					},
				}
				markFenceCreatedByController(sf)
				model.PatchIstioRevLabel(&sf.Labels, r.env.SelfResourceRev())
				if err = r.Client.Create(ctx, sf); err != nil {
					log.Errorf("create fence %s failed, %+v", nsName, err)
					return reconcile.Result{}, err
				}
				log.Infof("create fence succeed %s:%s in refreshFenceStatusOfService", sf.Namespace, sf.Name)
			}
		}
	} else if rev := model.IstioRevFromLabel(sf.Labels); !r.env.RevInScope(rev) {
		// check if svc needs auto fence created
		log.Errorf("existed fence %v istioRev %s but our rev %s, skip ...",
			nsName, rev, r.env.IstioRev())
	} else if isFenceCreatedByController(sf) {

		if svc == nil {
			log.Infof("svc is nil and delete svf %s:%s", sf.Namespace, sf.Name)
			if err := r.Client.Delete(ctx, sf); err != nil {
				log.Errorf("delete fence %s failed, %+v", nsName, err)
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}

		if fenced, err := r.isServiceFenced(ctx, svc); err != nil {
			return reconcile.Result{}, err
		} else if !fenced {
			log.Infof("svc is not fenced and delete svf %s:%s", svc.Namespace, svc.Name)
			if err := r.Client.Delete(ctx, sf); err != nil {
				log.Errorf("delete fence %s failed, %+v", nsName, err)
			}
		}
	}

	return ctrl.Result{}, nil
}

func isFenceCreatedByController(sf *lazyloadv1alpha1.ServiceFence) bool {
	if sf.Labels == nil {
		return false
	}
	return sf.Labels[LabelCreatedBy] == CreatedByFenceController
}

func markFenceCreatedByController(sf *lazyloadv1alpha1.ServiceFence) {
	if sf.Labels == nil {
		sf.Labels = map[string]string{}
	}
	sf.Labels = map[string]string{LabelCreatedBy: CreatedByFenceController}
}

var generatedWorkloadSFNameTpl = "%s-%s.workload.identity"

func (r *ServicefenceReconciler) handlePodAdd(ctx context.Context, obj interface{}) {
	r.handlePodUpdate(ctx, nil, obj)
}

func (r *ServicefenceReconciler) handlePodUpdate(ctx context.Context, _, obj interface{}) {

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}

	if r.inSpecialNs(pod.Namespace) {
		return
	}

	if pod.Status.PodIP == "" {
		return
	}

	var sfName string
	namespacedName := r.getServicefenceNameByIp(pod.Status.PodIP)
	if namespacedName.Namespace == "" || namespacedName.Name == "" {
		v, exist := pod.Labels[r.workloadFenceLabelKey]
		if !exist {
			// should not happend
			return
		}
		sfName = fmt.Sprintf(generatedWorkloadSFNameTpl, r.workloadFenceLabelKeyAlias, v)
		namespacedName = types.NamespacedName{Namespace: pod.Namespace, Name: sfName}
		// need create
		// add svc -> add sf
		sf := &lazyloadv1alpha1.ServiceFence{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespacedName.Name,
				Namespace: namespacedName.Namespace,
			},
			Spec: lazyloadv1alpha1.ServiceFenceSpec{
				Enable: true,
				WorkloadSelector: &lazyloadv1alpha1.WorkloadSelector{
					Labels: map[string]string{
						r.workloadFenceLabelKey: v,
					},
				},
			},
		}
		markFenceCreatedByController(sf)
		model.PatchIstioRevLabel(&sf.Labels, r.env.SelfResourceRev())
		if err := r.Client.Create(ctx, sf); err != nil {
			if errors.IsAlreadyExists(err) {
				r.appendIpToFence(namespacedName, pod.Status.PodIP)
				return
			}
			log.Errorf("create fence %s for workload selector by '%s=%s' failed: %s", namespacedName, r.workloadFenceLabelKey, v, err)
			// Todo: need retry
			return
		} else {
			r.appendIpToFence(namespacedName, pod.Status.PodIP)
		}
		log.Infof("create fence %s for workload selector by '%s=%s' ", namespacedName, r.workloadFenceLabelKey, v)
	}
}

func (r *ServicefenceReconciler) handlePodDelete(ctx context.Context, obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	if pod.Status.PodIP == "" {
		return
	}
	namespacedName := r.getServicefenceNameByIp(pod.Status.PodIP)
	if namespacedName.Namespace == "" || namespacedName.Name == "" {
		return
	}
	if r.delIpFromFence(namespacedName, pod.Status.PodIP) {
		sf := &lazyloadv1alpha1.ServiceFence{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespacedName.Namespace,
				Name:      namespacedName.Name},
		}
		if err := r.Client.Delete(ctx, sf); err != nil {
			log.Errorf("delete fence %s failed: %s", namespacedName, err)
			// Todo: need retry
			return
		}
	}
}

func (r *ServicefenceReconciler) NewPodController(client kubernetes.Interface, fenceLabelKeyAlias string) cache.Controller {
	ctx := context.Background()
	if fenceLabelKeyAlias == "" {
		fenceLabelKeyAlias = "app"
	}
	strs := strings.SplitN(fenceLabelKeyAlias, ":", 2)
	r.workloadFenceLabelKey = strs[0]
	r.workloadFenceLabelKeyAlias = "app"
	if len(strs) == 2 && strs[1] != "" {
		r.workloadFenceLabelKeyAlias = strs[1]
	}
	listOpts := metav1.ListOptions{LabelSelector: r.workloadFenceLabelKey}
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Pods("").List(ctx, listOpts)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Pods("").Watch(ctx, listOpts)
		},
	}
	_, controller := cache.NewInformer(lw, &corev1.Pod{}, 60*time.Second, cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { r.handlePodAdd(ctx, obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { r.handlePodUpdate(ctx, oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { r.handlePodDelete(ctx, obj) },
	})
	return controller
}

func (r *ServicefenceReconciler) getServicefenceNameByIp(ip string) types.NamespacedName {
	r.ipTofence.RLock()
	defer r.ipTofence.RUnlock()
	return r.ipTofence.Data[ip]
}

func (r *ServicefenceReconciler) appendIpToFence(namespacedName types.NamespacedName, ip string) {
	r.ipTofence.Lock()
	r.ipTofence.Data[ip] = namespacedName
	r.ipTofence.Unlock()
	r.fenceToIp.Lock()
	ips := r.fenceToIp.Data[namespacedName]
	if ips == nil {
		ips = map[string]struct{}{}
	}
	ips[ip] = struct{}{}
	r.fenceToIp.Data[namespacedName] = ips
	r.fenceToIp.Unlock()
}

func (r *ServicefenceReconciler) delIpFromFence(namespacedName types.NamespacedName, ip string) bool {
	r.ipTofence.Lock()
	delete(r.ipTofence.Data, ip)
	r.ipTofence.Unlock()
	r.fenceToIp.Lock()
	defer r.fenceToIp.Unlock()
	ips := r.fenceToIp.Data[namespacedName]
	delete(ips, ip)
	if ips == nil || len(ips) == 0 {
		delete(r.fenceToIp.Data, namespacedName)
		return true
	}
	r.fenceToIp.Data[namespacedName] = ips
	return false
}

// inSpecialNs slimeNamespace/istioNamespace/ClusterGsNamespace/kube-system is special, servicefence will not be generated
func (r *ServicefenceReconciler) inSpecialNs(ns string) bool {

	if ns == "kube-system" {
		return true
	}

	if r.env.Config != nil && r.env.Config.Global != nil {
		if ns == r.env.Config.Global.IstioNamespace || ns == r.env.Config.Global.SlimeNamespace {
			return true
		}
	}

	// global-sidecar may deploy different from slimeNamespace
	if r.cfg != nil {
		if ns == r.cfg.ClusterGsNamespace {
			return true
		}
	}

	return false
}
