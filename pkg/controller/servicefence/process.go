package servicefence

import (
	"context"
	"k8s.io/apimachinery/pkg/api/errors"
	microservicev1alpha1 "slime.io/slime/pkg/apis/microservice/v1alpha1"
	event_source "slime.io/slime/pkg/model/source"
)

func (r *ReconcileServiceFence) WatchSource(stop <-chan struct{}) {
	go func() {
		for {
			select {
			case <-stop:
				return
			case e := <-r.eventChan:
				switch e.EventType {
				case event_source.Update, event_source.Add:
					svf := &microservicev1alpha1.ServiceFence{}
					err := r.client.Get(context.TODO(), e.Loc, svf)
					if err != nil && errors.IsNotFound(err) {
						log.Info("ServiceFence Not Found, skip")
					}else if err != nil{
						log.Error(err,"can not get ServiceFence")
					}else{
						svf.Status.MetricStatus = e.Info
						err = r.client.Status().Update(context.TODO(),svf)
						if err != nil {
							log.Error(err,"can not update ServiceFence")
						}
					}
				}
			}
		}
	}()
}
