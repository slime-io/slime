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
	watchtools "k8s.io/client-go/tools/watch"
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
	l := log.WithField("reporter", "WatcherTrigger").WithField("function", "Start")

	t.watchersMap = make(map[watch.Interface]chan struct{})

	ctx := context.Background()
	for _, gvk := range t.kinds {
		gvr, _ := meta.UnsafeGuessKindToResource(gvk)
		dc := t.dynamicClient
		lw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return dc.Resource(gvr).List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return dc.Resource(gvr).Watch(ctx, options)
			},
		}
		_, _, watcher, _ := watchtools.NewIndexerInformerWatcher(lw, &unstructured.Unstructured{})
		t.watchersMap[watcher] = make(chan struct{})
		l.Infof("add watcher %s to watcher trigger", gvr.String())
	}

	for wat, channel := range t.watchersMap {
		go func(w watch.Interface, ch chan struct{}) {
			for {
				select {
				case <-ch:
					l.Debugf("stop a watcher")
					w.Stop()
					return
				case e, ok := <-w.ResultChan():
					l.Debugf("got watcher event: type %v, kind %v", e.Type, e.Object.GetObjectKind().GroupVersionKind())
					if !ok {
						l.Warningf("a result chan of watcher is closed, break process loop")
						return
					}
					object, ok := e.Object.(*unstructured.Unstructured)
					if !ok {
						l.Errorf("invalid type of object in watcher event")
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
