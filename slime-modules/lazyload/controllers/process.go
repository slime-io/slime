package controllers

import (
	"context"

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
					svf := &microservicev1alpha1.ServiceFence{}
					err := r.Client.Get(context.TODO(), e.Loc, svf)
					if err != nil && errors.IsNotFound(err) {
						r.Log.Info("ServiceFence Not Found, skip")
					} else if err != nil {
						r.Log.Error(err, "can not get ServiceFence")
					} else {
						svf.Status.MetricStatus = e.Info
						err = r.Client.Status().Update(context.TODO(), svf)
						if err != nil {
							r.Log.Error(err, "can not update ServiceFence")
						}
					}
				}
			}
		}
	}()
}
