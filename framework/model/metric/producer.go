package metric

import (
	"github.com/prometheus/common/log"
)

func NewProducer(config *ProducerConfig) {
	var source Source
	var wp *WatcherProducer
	var tp *TickerProducer

	// init source
	if config.EnablePrometheusSource {
		source = NewPrometheusSource(config.PrometheusSourceConfig)
	} else if config.EnableMockSource {
		source = NewMockSource()
	} else {
		source = NewAccessLogSource(config.AccessLogSourceConfig)
	}

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
		return
	}()
}
