package metric

import (
	log "github.com/sirupsen/logrus"
	"slime.io/slime/framework/model/trigger"
)

type TickerProducer struct {
	name                    string
	needUpdateMetricHandler func(event trigger.TickerEvent) QueryMap
	tickerTrigger           *trigger.TickerTrigger
	source                  Source
	MetricChan              chan Metric
	StopChan                chan struct{}
}

func NewTickerProducer(config TickerProducerConfig, source Source) *TickerProducer {
	tp := &TickerProducer{
		name:                    config.Name,
		needUpdateMetricHandler: config.NeedUpdateMetricHandler,
		tickerTrigger:           trigger.NewTickerTrigger(config.TickerTriggerConfig),
		source:                  source,
		MetricChan:              config.MetricChan,
		StopChan:                make(chan struct{}),
	}

	return tp
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

			// get metric
			metric, err := p.source.QueryMetric(queryMap)
			if err != nil {
				log.Errorf("%v", err)
				continue
			}

			// produce metric event
			p.MetricChan <- metric

		}
	}
}

func (p *TickerProducer) Start() {
	p.tickerTrigger.Start()
	p.source.Start()
}

func (p *TickerProducer) Stop() {
	p.tickerTrigger.Stop()
	p.StopChan <- struct{}{}
}
