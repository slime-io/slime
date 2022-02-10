package metric

import (
	data_accesslog "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	prometheusModel "github.com/prometheus/common/model"
	"slime.io/slime/framework/model/trigger"
)

type ProducerConfig struct {
	EnablePrometheusSource bool
	PrometheusSourceConfig PrometheusSourceConfig
	AccessLogSourceConfig  AccessLogSourceConfig
	EnableMockSource       bool
	EnableWatcherProducer  bool
	WatcherProducerConfig  WatcherProducerConfig
	EnableTickerProducer   bool
	TickerProducerConfig   TickerProducerConfig
	StopChan               <-chan struct{}
}

type WatcherProducerConfig struct {
	Name                    string
	NeedUpdateMetricHandler func(event trigger.WatcherEvent) QueryMap
	MetricChan              chan Metric
	WatcherTriggerConfig    trigger.WatcherTriggerConfig
}

type TickerProducerConfig struct {
	Name                    string
	NeedUpdateMetricHandler func(event trigger.TickerEvent) QueryMap
	MetricChan              chan Metric
	TickerTriggerConfig     trigger.TickerTriggerConfig
}

type PrometheusSourceConfig struct {
	Api       prometheus.API
	Convertor func(queryValue prometheusModel.Value) map[string]string
}

type AccessLogSourceConfig struct {
	ServePort                 string
	AccessLogConvertorConfigs []AccessLogConvertorConfig
}

type AccessLogConvertorConfig struct {
	Name    string // handler name
	Handler func(logEntry []*data_accesslog.HTTPAccessLogEntry) (map[string]map[string]string, error)
}
