package aggregate

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"reflect"
	"slime.io/slime/framework/model"
	"slime.io/slime/framework/model/source"
	"slime.io/slime/framework/model/watcher"
	"time"
	"fmt"
)

type MetricSource struct {
	sourcesMap       map[string]source.MetricSourceForAggregate                    // store all MetricSources with name
	gVKSourceMap     map[schema.GroupVersionKind][]source.MetricSourceForAggregate // gvk -> sources
	gVKStopChanMap   map[schema.GroupVersionKind]chan bool                         // gvk -> stopChan, stop channel for single gvk watcher
	stopChan         chan bool                                                     // stop channel for MetricSource self
	watcherEventChan <-chan model.WatcherEvent
	restConfig       rest.Config
}

// global variable
var metricSource = &MetricSource{}

func init() {
	log.Infof("aggregate metricSource inits successfully")
	metricSource.sourcesMap = make(map[string]source.MetricSourceForAggregate)
	metricSource.gVKSourceMap = make(map[schema.GroupVersionKind][]source.MetricSourceForAggregate)
	metricSource.gVKStopChanMap = make(map[schema.GroupVersionKind]chan bool)
	metricSource.stopChan = make(chan bool)
}

func Add(m source.MetricSourceForAggregate) error {
	// stop watchers if existing gvk watcher channel
	if len(metricSource.gVKStopChanMap) > 0 {
		metricSource.StopWatchers()
	}

	// update SourcesMap
	if _, ok := metricSource.sourcesMap[m.Name()]; ok {
		return errors.New("source already exists, nothing to change")
	}
	metricSource.sourcesMap[m.Name()] = m
	// update RestConfig
	if reflect.DeepEqual(metricSource.restConfig, rest.Config{}) {
		log.Infof("init metricSource.restConfig")
		metricSource.restConfig = m.RestConfig()
	}
	// update GVKSourceMap and GVKStopChanMap
	metricSource.MapAdd(m)
	metricSource.StartWatchers()
	return nil
}

func Remove(m source.MetricSourceForAggregate) error {
	// stop watchers if existing gvk watcher channel
	if len(metricSource.gVKStopChanMap) > 0 {
		metricSource.StopWatchers()
	}
	// update SourcesMap
	if _, ok := metricSource.sourcesMap[m.Name()]; ok {
		delete(metricSource.sourcesMap, m.Name())
	} else {
		return errors.New(fmt.Sprintf("metric source %s not exists", m.Name()))
	}
	// check if this is the last metric source
	if len(metricSource.sourcesMap) == 0 {
		log.Infof("no metric source now, will return")
		return nil
	}
	// update GVKSourceMap and GVKStopChanMap
	metricSource.MapRemove(m)
	metricSource.StartWatchers()
	return nil
}

func (ms *MetricSource) MapAdd(m source.MetricSourceForAggregate) {
	for _, gvk := range m.GVKs() {
		if _, ok := ms.gVKSourceMap[gvk]; !ok {
			log.Debugf("add new key %s to aggregate metric source gVKSourceMap", gvk.String())
			// update GVKSourceMap
			ms.gVKSourceMap[gvk] = []source.MetricSourceForAggregate{m}
			// update GVKStopChanMap
			ms.gVKStopChanMap[gvk] = make(chan bool)
		} else {
			log.Debugf("update key %s in aggregate metric source gVKSourceMap", gvk.String())
			// only update gVKSourceMap, GVKStopChanMap[gvk] already exists
			ms.gVKSourceMap[gvk] = append(ms.gVKSourceMap[gvk], m)
		}
	}
}

func (ms *MetricSource) MapRemove(m source.MetricSourceForAggregate) {
	for _, gvk := range m.GVKs() {
		if s, ok := ms.gVKSourceMap[gvk]; !ok {
			log.Warningf("aggregate metric source gVKSourceMap does not contain key %s", gvk.String())
		} else {
			for i, v := range s {
				if v.Name() == m.Name() {
					if len(s) == 1 {
						// the last user of this gvk, delete key
						delete(ms.gVKStopChanMap, gvk)
						delete(ms.gVKSourceMap, gvk)
					} else {
						// only update GVKSourceMap, GVKStopChanMap[gvk] still has other users
						s = append(s[:i], s[i+1:]...)
					}
					log.Debugf("successfuly remove gvk %s of metric source %s", gvk.String(), m.Name())
					break
				}
			}
			log.Errorf("failed to remove gvk %s of metric source %s", gvk.String(), m.Name())
		}
	}
}

func (ms *MetricSource) StopWatchers() {
	for _, ch := range ms.gVKStopChanMap {
		go func() {
			ch <- true
		}()
	}
	log.Infof("Aggregate MetricSource sends stop signals to all watchers")
}

func (ms *MetricSource) StartWatchers() {
	// start MetricWatcher
	var err error
	ms.watcherEventChan, err = watcher.Register(ms)
	if err != nil {
		log.Errorf("MetricSource registers to MetricWatcher error: %v", err)
		return
	}
	ms.ConsumeEvent()
}

// 消费WatcherEvent: 提炼出gvk+nn信息，找出关联的modules
func (ms *MetricSource) ConsumeEvent() {
	// check sources number
	go func() {
		for {
			if len(ms.sourcesMap) == 0 {
				log.Infof("mo metric source in sourcesMap, stop consuming watcherevent...")
				ms.stopChan <- true
				return
			}
		}
	}()

	// create fake watcher event from ticker
	ticker := time.NewTicker(30 * time.Second)

	// consume event
	go func() {
		for {
			select {
			case <-ms.stopChan:
				log.Infof("stop consuming watcherevent")
				return
			case we, ok := <-ms.watcherEventChan:
				log.Infof("Aggregated MetricSource got an watcherEvent, gvk: %s, nn: %s", we.GVK.String(), we.NN.String())
				if !ok {
					log.Warningf("Aggregated MetricSource WatcherEventChan closed, break process loop")
					return
				}
				// deliver watcher event to related sources
				for _, source := range ms.gVKSourceMap[we.GVK] {
					log.Infof("Aggregated MetricSource send an watcherEvent to MetricSource, gvk: %s, nn: %s, source: %s", we.GVK.String(), we.NN.String(), source.Name())
					source.Notify(we)
				}
			case <-ticker.C:
				log.Infof("Aggregate MetricSource generates fake watcher event")
				for _, source := range ms.sourcesMap {
					source.Notify(model.WatcherEvent{})
				}

			}
		}
	}()
}

func (ms *MetricSource) GVKStopChanMap() map[schema.GroupVersionKind]chan bool {
	return ms.gVKStopChanMap
}

func (ms *MetricSource) RestConfig() *rest.Config {
	return &ms.restConfig
}
