package util

import (
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

func ListWatcher(ctx context.Context, lw *cache.ListWatch) watch.Interface {
	ch := make(chan watch.Event)
	go func() {
		_, err := watchtools.ListWatchUntil(ctx, lw, func(event watch.Event) (bool, error) {
			ch <- event
			return false, nil
		})
		if err != nil {
			log.Errorf("ListWatcher got err %v", err)
		}
	}()
	return watch.NewProxyWatcher(ch)
}
