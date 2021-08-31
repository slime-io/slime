package controllers

import (
	"context"
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/api/errors"
	event_source "slime.io/slime/slime-framework/model/source"
	microservicev1alpha1 "slime.io/slime/slime-modules/lazyload/api/v1alpha1"
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
	svf := &microservicev1alpha1.ServiceFence{}
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
