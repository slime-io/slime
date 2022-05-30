package controllers

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (r *ServicefenceReconciler) WatchMetric() {
	log := log.WithField("reporter", "ServicefenceReconciler").WithField("function", "WatchMetric")
	log.Infof("start watching metric")

	for {
		select {
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

	// use updateVisitedHostStatus to update svf.spec and svf.status
	sf.Status.MetricStatus = value
	diff := r.updateVisitedHostStatus(sf)
	r.recordVisitor(sf, diff)

	if sf.Spec.Enable {
		if err := r.refreshSidecar(sf); err != nil {
			// XXX return err?
			log.Errorf("refresh sidecar %v met err: %v", req.NamespacedName, err)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ServicefenceReconciler) isNsFenced(ns *corev1.Namespace) bool {
	if ns != nil && ns.Labels != nil {
		switch ns.Labels[LabelServiceFenced] {
		case ServiceFencedTrue:
			return true
		case ServiceFencedFalse:
			return false
		}
	}
	return r.cfg.DefaultFence
}

func (r *ServicefenceReconciler) isServiceFenced(ctx context.Context, svc *corev1.Service) bool {
	var svcLabel string
	if svc.Labels != nil {
		svcLabel = svc.Labels[LabelServiceFenced]
	}

	switch svcLabel {
	case ServiceFencedFalse:
		return false
	case ServiceFencedTrue:
		return true
	default:
		if r.staleNamespaces[svc.Namespace] {
			ns := &corev1.Namespace{}
			if err := r.Client.Get(ctx, types.NamespacedName{
				Namespace: "",
				Name:      svc.Namespace,
			}, ns); err != nil {
				if errors.IsNotFound(err) {
					ns = nil
				} else {
					ns = nil
					log.Errorf("fail to get ns: %s", svc.Namespace)
				}
			}

			if ns != nil {
				return r.isNsFenced(ns)
			}
		}
		return r.enabledNamespaces[svc.Namespace]
	}
}

func (r *ServicefenceReconciler) ReconcileService(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.TODO()

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	return r.refreshFenceStatusOfService(ctx, nil, req.NamespacedName)
}

func (r *ServicefenceReconciler) ReconcileNamespace(req ctrl.Request) (ret ctrl.Result, err error) {
	ctx := context.TODO()

	// Fetch the namespace instance
	ns := &corev1.Namespace{}
	err = r.Client.Get(ctx, req.NamespacedName, ns)

	if ns.Name == r.env.Config.Global.IstioNamespace || ns.Name == r.env.Config.Global.SlimeNamespace {
		log.Debugf("auto fence does not apply to istio and slime namespace, skip")
		return reconcile.Result{}, nil
	}

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	defer func() {
		if err == nil {
			delete(r.staleNamespaces, req.Name)
		}
	}()

	if err != nil {
		if errors.IsNotFound(err) {
			ns = nil
			delete(r.enabledNamespaces, req.Name)
			return reconcile.Result{}, nil // do not process deletion ...
		} else {
			log.Errorf("get namespace %s error, %+v", req.NamespacedName, err)
			return reconcile.Result{}, err
		}
	}

	nsFenced := r.isNsFenced(ns)

	if nsFenced == r.enabledNamespaces[req.Name] {
		return reconcile.Result{}, nil
	} else {
		prev := r.enabledNamespaces[req.Name]
		r.enabledNamespaces[req.Name] = nsFenced
		defer func() {
			if err != nil {
				r.enabledNamespaces[req.Name] = prev // restore, leave to re-process next time
				r.staleNamespaces[req.Name] = true
			}
		}()
	}

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
		if svc != nil && r.isServiceFenced(ctx, svc) {
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
			model.PatchIstioRevLabel(&sf.Labels, r.env.IstioRev())
			if err = r.Client.Create(ctx, sf); err != nil {
				log.Errorf("create fence %s failed, %+v", nsName, err)
				return reconcile.Result{}, err
			}
		}
	} else if rev := model.IstioRevFromLabel(sf.Labels); !r.env.RevInScope(rev) {
		// check if svc needs auto fence created
		log.Errorf("existed fence %v istioRev %s but our rev %s, skip ...",
			nsName, rev, r.env.IstioRev())
	} else if isFenceCreatedByController(sf) && (svc == nil || !r.isServiceFenced(ctx, svc)) {
		if err := r.Client.Delete(ctx, sf); err != nil {
			log.Errorf("delete fence %s failed, %+v", nsName, err)
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
