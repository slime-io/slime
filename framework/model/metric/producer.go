package metric

import log "github.com/sirupsen/logrus"

func NewProducer(config *ProducerConfig, source Source) {
	var wp *WatcherProducer
	var tp *TickerProducer

	if config.EnableWatcherProducer {
		log.Debugf("lazyload: watch producer begin")
		wp = NewWatcherProducer(config.WatcherProducerConfig, source)
		wp.Start()
		go wp.HandleWatcherEvent()
	}

	if config.EnableTickerProducer {
		log.Debugf("lazyload: ticker producer begin")
		tp = NewTickerProducer(config.TickerProducerConfig, source)
		tp.Start()
		go tp.HandleTickerEvent()
	}

	// stop producers
	go func() {
		<-config.StopChan
		if config.EnableWatcherProducer {
			wp.Stop()
		}
		if config.EnableTickerProducer {
			tp.Stop()
		}
		log.Infof("all producers stopped")
	}()
}

func NewSource(config *ProducerConfig) Source {
	// init source
	var source Source
	switch {
	case config.EnablePrometheusSource:
		source = NewPrometheusSource(config.PrometheusSourceConfig)
	case config.EnableMockSource:
		source = NewMockSource()
	default:
		source = NewAccessLogSource(config.AccessLogSourceConfig)
	}
	return source
}
