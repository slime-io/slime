package metric

import (
	log "github.com/sirupsen/logrus"
	"slime.io/slime/framework/model/trigger"
)

type WatcherProducer struct {
	name                    string
	needUpdateMetricHandler func(event trigger.WatcherEvent) QueryMap
	watcherTrigger          *trigger.WatcherTrigger
	prometheusSource        *PrometheusSource
	MetricChan              chan Metric
	StopChan                chan struct{}
}

func NewWatcherProducer(config WatcherProducerConfig) *WatcherProducer {
	return &WatcherProducer{
		name:                    config.Name,
		needUpdateMetricHandler: config.NeedUpdateMetricHandler,
		watcherTrigger:          trigger.NewWatcherTrigger(config.WatcherTriggerConfig),
		prometheusSource:        NewPrometheusSource(config.PrometheusSourceConfig),
		MetricChan:              config.MetricChan,
		StopChan:                make(chan struct{}),
	}
}

func (p *WatcherProducer) HandleWatcherEvent() {
	log := log.WithField("reporter", "WatcherProvider").WithField("function", "HandleTriggerEvent")
	for {
		select {
		case <-p.StopChan:
			log.Infof("watcher producer exited")
			return
		// handle watcher event
		case event, ok := <-p.watcherTrigger.EventChan():
			if !ok {
				log.Warningf("watcher event channel closed, break process loop")
				return
			}
			log.Debugf("got watcher trigger event")
			// reconciler callback
			queryMap := p.needUpdateMetricHandler(event)
			if queryMap == nil {
				log.Debugf("queryMap is nil, finish")
				continue
			}

			// get metric material
			metric, err := p.prometheusSource.QueryMetric(queryMap)
			if err != nil {
				log.Errorf("%v", err)
				continue
			}

			// produce material event
			p.MetricChan <- metric
		}
	}
}

func (p *WatcherProducer) Start() {
	p.watcherTrigger.Start()
}

func (p *WatcherProducer) Stop() {
	p.watcherTrigger.Stop()
	p.StopChan <- struct{}{}
}
