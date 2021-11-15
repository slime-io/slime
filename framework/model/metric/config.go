package metric

import (
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	prometheusModel "github.com/prometheus/common/model"
	"slime.io/slime/framework/model/trigger"
)

type ProducerConfig struct {
	EnableWatcherProducer bool
	WatcherProducerConfig WatcherProducerConfig
	EnableTickerProducer  bool
	TickerProducerConfig  TickerProducerConfig
	StopChan              <-chan struct{}
}

type WatcherProducerConfig struct {
	Name                    string
	NeedUpdateMetricHandler func(event trigger.WatcherEvent) QueryMap
	MetricChan              chan Metric
	WatcherTriggerConfig    trigger.WatcherTriggerConfig
	PrometheusSourceConfig  PrometheusSourceConfig
}

type TickerProducerConfig struct {
	Name                    string
	NeedUpdateMetricHandler func(event trigger.TickerEvent) QueryMap
	MetricChan              chan Metric
	TickerTriggerConfig     trigger.TickerTriggerConfig
	PrometheusSourceConfig  PrometheusSourceConfig
}

type PrometheusSourceConfig struct {
	Api       prometheus.API
	Convertor func(queryValue prometheusModel.Value) map[string]string
}

type AccessLogSourceConfig struct {
}
