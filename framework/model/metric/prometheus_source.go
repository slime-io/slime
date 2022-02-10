package metric

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	prometheusModel "github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

type PrometheusSource struct {
	api       prometheus.API
	convertor func(queryValue prometheusModel.Value) map[string]string
}

func NewPrometheusSource(config PrometheusSourceConfig) *PrometheusSource {
	ps := &PrometheusSource{
		api:       config.Api,
		convertor: defaultConvertor,
	}
	if config.Convertor != nil {
		ps.convertor = config.Convertor
	}
	return ps
}

func (ps *PrometheusSource) Start() error {
	return nil
}

func (ps *PrometheusSource) QueryMetric(queryMap QueryMap) (Metric, error) {
	log := log.WithField("reporter", "PrometheusSource").WithField("function", "QueryMetric")

	metric := make(map[string][]Result)

	for meta, handlers := range queryMap {
		if len(handlers) == 0 {
			continue
		}

		for _, handler := range handlers {
			queryValue, w, e := ps.api.Query(context.Background(), handler.Query, time.Now())
			if e != nil {
				log.Debugf("failed get metric from prometheus, name: %s, query: %s, error: %+v", handler.Name, handler.Query, e)
				return nil, errors.New(fmt.Sprintf("failed to get metric from prometheus, error: %+v", e))
			} else if w != nil {
				log.Debugf("failed get metric from prometheus, name: %s, query: %s, warning: %s", handler.Name, handler.Query, strings.Join(w, ";"))
				return nil, errors.New(fmt.Sprintf("failed to get metric from prometheus, warning: %s", strings.Join(w, ";")))
			}
			result := Result{
				Name:  handler.Name,
				Value: ps.convertor(queryValue),
			}
			metric[meta] = append(metric[meta], result)
		}

	}
	log.Debugf("successfully get metric from prometheus")
	return metric, nil
}

func defaultConvertor(qv prometheusModel.Value) map[string]string {
	result := make(map[string]string)

	switch qv.Type() {
	case prometheusModel.ValScalar:
		scalar := qv.(*prometheusModel.Scalar)
		result[scalar.Value.String()] = scalar.Timestamp.String()
	case prometheusModel.ValVector:
		vector := qv.(prometheusModel.Vector)
		for _, vx := range vector {
			result[vx.Metric.String()] = vx.Value.String()
		}
	case prometheusModel.ValMatrix:
		matrix := qv.(prometheusModel.Matrix)
		for _, sampleStream := range matrix {
			v := ""
			for _, sp := range sampleStream.Values {
				v = v + sp.String()
			}
			result[sampleStream.Metric.String()] = v
		}
	case prometheusModel.ValString:
		str := qv.(*prometheusModel.String)
		result[str.Value] = str.Timestamp.String()
	}

	return result
}
