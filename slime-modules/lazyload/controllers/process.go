package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/errors"

	event_source "slime.io/slime/slime-framework/model/source"
	lazyloadv1alpha1 "slime.io/slime/slime-modules/lazyload/api/v1alpha1"
)

const (
	LabelServiceFenced = "slime.io/serviceFenced"
	ServiceFencedTrue  = "true"
	ServiceFencedFalse = "false"
)

func (r *ServicefenceReconciler) WatchSource(stop <-chan struct{}) {
	go func() {
		for {
			select {
			case <-stop:
				return
			case e := <-r.eventChan:
				switch e.EventType {
				case event_source.Update, event_source.Add:
					if _, err := r.Refresh(reconcile.Request{NamespacedName: e.Loc}, e.Info); err != nil {
						fmt.Printf("error:%v", err)
					}
				}
			}
		}
	}()
}

func (r *ServicefenceReconciler) Refresh(request reconcile.Request, args map[string]string) (reconcile.Result, error) {
	svf := &lazyloadv1alpha1.ServiceFence{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, svf)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("ServiceFence Not Found, skip")
		return reconcile.Result{}, nil
	} else if err != nil {
		r.Log.Error(err, "can not get ServiceFence")
		return reconcile.Result{}, err
	} else {
		if svf.Spec.Enable {
			r.refreshSidecar(svf)
		}

		svf.Status.MetricStatus = args
		err = r.Client.Status().Update(context.TODO(), svf)
		if err != nil {
			r.Log.Error(err, "can not update ServiceFence")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}
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
					r.Log.Error(err, "fail to get ns", "ns", svc.Namespace)
				}
			}

			if ns != nil && ns.Labels != nil {
				return ns.Labels[LabelServiceFenced] == ServiceFencedTrue
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

	// Fetch the Service instance
	ns := &corev1.Namespace{}
	err = r.Client.Get(ctx, req.NamespacedName, ns)

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
			r.Log.Error(err, "get namespace error", "ns", req.NamespacedName)
			return reconcile.Result{}, err
		}
	}

	var nsLabel string
	if ns.Labels != nil {
		nsLabel = ns.Labels[LabelServiceFenced]
	}

	nsFenced := nsLabel == ServiceFencedTrue
	if nsFenced == r.enabledNamespaces[req.Name] {
		return reconcile.Result{}, nil
	} else if nsFenced {
		r.enabledNamespaces[req.Name] = true
		defer func() {
			if err != nil {
				delete(r.enabledNamespaces, req.Name) // restore, leave to re-process next time
				r.staleNamespaces[req.Name] = true
			}
		}()
	}

	// refresh service fenced status
	services := &corev1.ServiceList{}
	if err = r.Client.List(ctx, services, client.InNamespace(req.Name)); err != nil {
		r.Log.Error(err, "list services failed", "ns", req.Name)
		return reconcile.Result{}, err
	}

	for _, svc := range services.Items {
		if ret, err = r.refreshFenceStatusOfService(ctx, &svc, types.NamespacedName{}); err != nil {
			r.Log.Error(err, "refreshFenceStatusOfService services failed", "svc", svc.Name)
			return ret, err
		}
	}

	return ctrl.Result{}, nil
}

// refreshFenceStatusOfService caller should held the reconcile lock.
func (r *ServicefenceReconciler) refreshFenceStatusOfService(ctx context.Context, svc *corev1.Service, nsName types.NamespacedName) (reconcile.Result, error) {
	if svc == nil {
		// Fetch the Service instance
		svc = &corev1.Service{}
		err := r.Client.Get(ctx, nsName, svc)
		if err != nil {
			if errors.IsNotFound(err) {
				svc = nil
			} else {
				r.Log.Error(err, "get service error", "svc", nsName)
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
			r.Log.Error(err, "get serviceFence error", "fence", nsName)
			return reconcile.Result{}, err
		}
	}

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	if sf == nil {
		if svc != nil && r.isServiceFenced(ctx, svc) {
			sf = &lazyloadv1alpha1.ServiceFence{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      svc.Name,
					Namespace: svc.Namespace,
				},
			}
			if err := r.Client.Create(ctx, sf); err != nil {
				r.Log.Error(err, "create fence failed", "fence", nsName)
				return reconcile.Result{}, err
			}
		}
	} else if svc == nil || !r.isServiceFenced(ctx, svc) {
		if err := r.Client.Delete(ctx, sf); err != nil {
			r.Log.Error(err, "delete fence failed", "fence", nsName)
		}
	}

	return ctrl.Result{}, nil
}
