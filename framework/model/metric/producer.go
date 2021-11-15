package metric

import (
	"github.com/prometheus/common/log"
)

func NewProducer(config *ProducerConfig) {

	var wp *WatcherProducer
	var tp *TickerProducer

	if config.EnableWatcherProducer {
		wp = NewWatcherProducer(config.WatcherProducerConfig)
		wp.Start()
		go wp.HandleWatcherEvent()
	}

	if config.EnableTickerProducer {
		tp = NewTickerProducer(config.TickerProducerConfig)
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
