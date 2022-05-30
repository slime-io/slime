package controllers

import (
	"context"
	stderrors "errors"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"slime.io/slime/framework/util"
)

func newSvcCache(clientSet *kubernetes.Clientset) (*NsSvcCache, *LabelSvcCache, error) {
	log := log.WithField("function", "newLabelSvcCache")
	nsSvcCache := &NsSvcCache{Data: map[string]map[string]struct{}{}}
	labelSvcCache := &LabelSvcCache{Data: map[LabelItem]map[string]struct{}{}}

	// init labelSvcCache
	services, err := clientSet.CoreV1().Services("").List(metav1.ListOptions{})
	if err != nil {
		return nil, nil, stderrors.New("failed to get service list")
	}

	for _, service := range services.Items {
		ns := service.GetNamespace()
		name := service.GetName()
		svc := ns + "/" + name
		if nsSvcCache.Data[ns] == nil {
			nsSvcCache.Data[ns] = make(map[string]struct{})
		}
		nsSvcCache.Data[ns][svc] = struct{}{}
		for k, v := range service.GetLabels() {
			label := LabelItem{
				Name:  k,
				Value: v,
			}
			if labelSvcCache.Data[label] == nil {
				labelSvcCache.Data[label] = make(map[string]struct{})
			}
			labelSvcCache.Data[label][svc] = struct{}{}
		}
	}

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

		}
	}()

	return nsSvcCache, labelSvcCache, nil
}
