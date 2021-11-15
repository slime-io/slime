package metric

import (
	log "github.com/sirupsen/logrus"
	"slime.io/slime/framework/model/trigger"
)

type TickerProducer struct {
	name                    string
	needUpdateMetricHandler func(event trigger.TickerEvent) QueryMap
	tickerTrigger           *trigger.TickerTrigger
	prometheusSource        *PrometheusSource
	MetricChan              chan Metric
	StopChan                chan struct{}
}

func NewTickerProducer(config TickerProducerConfig) *TickerProducer {
	return &TickerProducer{
		name:                    config.Name,
		needUpdateMetricHandler: config.NeedUpdateMetricHandler,
		tickerTrigger:           trigger.NewTickerTrigger(config.TickerTriggerConfig),
		prometheusSource:        NewPrometheusSource(config.PrometheusSourceConfig),
		MetricChan:              config.MetricChan,
		StopChan:                make(chan struct{}),
	}
}

func (p *TickerProducer) HandleTickerEvent() {
	log := log.WithField("reporter", "TickerProducer").WithField("function", "HandleTriggerEvent")
	for {
		select {
		case <-p.StopChan:
			log.Infof("ticker producer exited")
			return
		// handle ticker event
		case event, ok := <-p.tickerTrigger.EventChan():
			if !ok {
				log.Warningf("ticker event channel closed, break process loop")
				return
			}

			// reconciler callback
			queryMap := p.needUpdateMetricHandler(event)
			if queryMap == nil {
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

func (p *TickerProducer) Start() {
	p.tickerTrigger.Start()
}

func (p *TickerProducer) Stop() {
	p.tickerTrigger.Stop()
	p.StopChan <- struct{}{}
}
