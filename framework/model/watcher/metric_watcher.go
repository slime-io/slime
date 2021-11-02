package watcher

import (
	"context"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"slime.io/slime/framework/model"
	"slime.io/slime/framework/model/source"
	"slime.io/slime/framework/util"
)

type MetricWatcher struct {
	WatchersMap      map[watch.Interface]chan bool
	WatcherEventChan chan model.WatcherEvent
}

// Register MetricWatcher init
func Register(ms source.AggregateMetricSource) (<-chan model.WatcherEvent, error) {
	watcherEventChan := make(chan model.WatcherEvent)
	WatchersMap := make(map[watch.Interface]chan bool)
	mw := &MetricWatcher{
		WatchersMap: WatchersMap,
		WatcherEventChan: watcherEventChan,
	}
	// gvk -> watcher
	dc, err := dynamic.NewForConfig(ms.RestConfig())
	for gvk, ch := range ms.GVKStopChanMap() {
		if err != nil {
			return nil, err
		}
		// create watcher for dynamic client
		gvr, _ := meta.UnsafeGuessKindToResource(gvk)
		lw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return dc.Resource(gvr).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return dc.Resource(gvr).Watch(options)
			},
		}
		watcher := util.ListWatcher(context.Background(), lw)

		mw.WatchersMap[watcher] = ch
		log.Debugf("add %s watcher to mw.WatchersMap", gvr.String())
	}

	mw.GenerateEvent()

	return mw.WatcherEventChan, nil
}

func (mw *MetricWatcher) GenerateEvent() {
	for w, ch := range mw.WatchersMap {
		go func() {
			for {
				select {
				case <-ch:
					log.Infof("stop watcher GenerateEvent")
					return
				case e, ok := <-w.ResultChan():
					log.Infof("watcher got event with kind %s", e.Type)
					if !ok {
						log.Warningf("One result chan of MetricWatcher closed, break process loop")
						return
					}
					// Event -> WatcherEvent
					//object, ok := e.Object.(metav1.Object)
					object, ok := e.Object.(*unstructured.Unstructured)
					if !ok {
						log.Errorf("invalid type of object in watcher event\n")
						continue
					}
					we := model.WatcherEvent{
						GVK: e.Object.GetObjectKind().GroupVersionKind(),
						NN: types.NamespacedName{
							Name:      object.GetName(),
							Namespace: object.GetNamespace(),
						},
					}
					log.Debugf("GenerateEvent: object gvk: %s\n", we.GVK.String())
					log.Debugf("GenerateEvent: object nn: %s\n", we.NN.String())
					// send WatcherEvent to MetricSource
					mw.WatcherEventChan <- we
				}
			}
		}()
	}
}
