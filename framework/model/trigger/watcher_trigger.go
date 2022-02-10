package trigger

import (
	"context"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"slime.io/slime/framework/util"
)

type WatcherTrigger struct {
	kinds         []schema.GroupVersionKind
	dynamicClient dynamic.Interface
	watchersMap   map[watch.Interface]chan struct{}
	eventChan     chan WatcherEvent
}

type WatcherEvent struct {
	GVK schema.GroupVersionKind
	NN  types.NamespacedName
}

type WatcherTriggerConfig struct {
	Kinds         []schema.GroupVersionKind
	DynamicClient dynamic.Interface
	EventChan     chan WatcherEvent
}

func NewWatcherTrigger(config WatcherTriggerConfig) *WatcherTrigger {
	return &WatcherTrigger{
		kinds:         config.Kinds,
		dynamicClient: config.DynamicClient,
		eventChan:     config.EventChan,
	}
}

func (t *WatcherTrigger) Start() {
	log := log.WithField("reporter", "WatcherTrigger").WithField("function", "Start")

	t.watchersMap = make(map[watch.Interface]chan struct{})

	for _, gvk := range t.kinds {
		gvr, _ := meta.UnsafeGuessKindToResource(gvk)
		dc := t.dynamicClient
		lw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return dc.Resource(gvr).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return dc.Resource(gvr).Watch(options)
			},
		}
		watcher := util.ListWatcher(context.Background(), lw)
		t.watchersMap[watcher] = make(chan struct{})
		log.Infof("add watcher %s to watcher trigger", gvr.String())
	}

	for wat, channel := range t.watchersMap {
		go func(w watch.Interface, ch chan struct{}) {
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						log.Debugf("stop a watcher")
						return
					}
				case e, ok := <-w.ResultChan():
					log.Debugf("got watcher event: type %v, kind %v", e.Type, e.Object.GetObjectKind().GroupVersionKind())
					if !ok {
						log.Warningf("a result chan of watcher is closed, break process loop")
						return
					}
					object, ok := e.Object.(*unstructured.Unstructured)
					if !ok {
						log.Errorf("invalid type of object in watcher event")
						continue
					}
					event := WatcherEvent{
						GVK: e.Object.GetObjectKind().GroupVersionKind(),
						NN: types.NamespacedName{
							Name:      object.GetName(),
							Namespace: object.GetNamespace(),
						},
					}
					t.eventChan <- event
					// log.Debugf("sent watcher event to controller: type %s, kind %s", e.Type, e.Object.GetObjectKind().GroupVersionKind().String())
				}
			}
		}(wat, channel)
	}
}

func (t *WatcherTrigger) Stop() {
	for _, ch := range t.watchersMap {
		close(ch)
	}
}

func (t *WatcherTrigger) EventChan() <-chan WatcherEvent {
	return t.eventChan
}
