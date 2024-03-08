package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"

	"slime.io/slime/framework/util"
)

const (
	UnitSeconds      = "s"
	UnitMilliseconds = "ms"

	UnitOne = ""
)

func WithUnit(unit string) api.InstrumentOption {
	return api.WithUnit(unit)
}

var SubModulesCount = NewGauge(
	"framework",
	"submodules_count",
	"total number of submodules",
)

func NewExporter() (http.Handler, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(provider)
	return promhttp.Handler(), nil
}

type Label struct {
	key attribute.Key
}

func (l Label) Value(value string) attribute.KeyValue {
	return attribute.KeyValue{
		Key:   l.key,
		Value: attribute.StringValue(value),
	}
}

func MustCreateLabel(key string) Label {
	return Label{key: attribute.Key(key)}
}

type MetricSum struct {
	ctx         context.Context
	counter     api.Float64UpDownCounter
	opts        []api.AddOption
	initializer func() api.Float64UpDownCounter
}

func NewSum(module string, name string, help string) *MetricSum {
	sum := &MetricSum{
		ctx: context.Background(),
		initializer: func() api.Float64UpDownCounter {
			counter, _ := otel.GetMeterProvider().Meter(util.MetricPrefix).
				Float64UpDownCounter(fmt.Sprintf("%s_%s", module, name), api.WithDescription(help))
			return counter
		},
	}

	return sum
}

func (sum *MetricSum) With(lbls ...attribute.KeyValue) *MetricSum {
	opts := make([]api.AddOption, 0, len(sum.opts)+len(lbls))
	opts = append(opts, sum.opts...)
	return &MetricSum{
		ctx:         sum.ctx,
		counter:     sum.counter,
		opts:        append(opts, api.WithAttributes(lbls...)),
		initializer: sum.initializer,
	}
}

func (sum *MetricSum) Increment() {
	if sum.counter == nil {
		sum.counter = sum.initializer()
	}
	sum.counter.Add(sum.ctx, 1, sum.opts...)
}

func (sum *MetricSum) Decrement() {
	if sum.counter == nil {
		sum.counter = sum.initializer()
	}
	sum.counter.Add(sum.ctx, -1, sum.opts...)
}

func WithHistogramBounds(bounds ...float64) api.Float64HistogramOption {
	return api.WithExplicitBucketBoundaries(bounds...)
}

type MetricHistogram struct {
	ctx         context.Context
	histogram   api.Float64Histogram
	opts        []api.RecordOption
	initializer func() api.Float64Histogram
}

func NewHistogram(module string, name string, help string, opts ...api.Float64HistogramOption) *MetricHistogram {
	histogram := &MetricHistogram{
		ctx: context.Background(),
		initializer: func() api.Float64Histogram {
			opts = append(opts, api.WithDescription(help))
			histogram, _ := otel.GetMeterProvider().Meter(util.MetricPrefix).
				Float64Histogram(fmt.Sprintf("%s_%s", module, name), opts...)
			return histogram
		},
	}

	return histogram
}

func (h *MetricHistogram) With(lbls ...attribute.KeyValue) *MetricHistogram {
	opts := make([]api.RecordOption, 0, len(h.opts)+len(lbls))
	opts = append(opts, h.opts...)
	return &MetricHistogram{
		ctx:         h.ctx,
		histogram:   h.histogram,
		opts:        append(opts, api.WithAttributes(lbls...)),
		initializer: h.initializer,
	}
}

func (h *MetricHistogram) Record(value float64) {
	if h.histogram == nil {
		h.histogram = h.initializer()
	}
	h.histogram.Record(h.ctx, value, h.opts...)
}

type MetricGauge struct {
	sync.Once
	gauge       api.Float64ObservableGauge
	opts        []api.ObserveOption
	initializer func() api.Float64ObservableGauge

	derived map[string]*MetricGauge

	curValue atomic.Value // store the current float64 value
}

func NewGauge(module string, name string, help string) *MetricGauge {
	gauge := &MetricGauge{
		derived:  make(map[string]*MetricGauge),
		curValue: atomic.Value{},
	}
	gauge.curValue.Store(float64(0))
	gauge.initializer = func() api.Float64ObservableGauge {
		g, _ := otel.GetMeterProvider().Meter(util.MetricPrefix).
			Float64ObservableGauge(fmt.Sprintf("%s_%s", module, name), api.WithDescription(help))
		return g
	}
	return gauge
}

func (g *MetricGauge) With(lbls ...attribute.KeyValue) *MetricGauge {
	if g.gauge == nil {
		g.gauge = g.initializer()
	}
	if len(lbls) == 0 {
		return g
	}

	key := buildKeyFromLabels(lbls)
	if og, ok := g.derived[key]; ok {
		return og
	}
	opts := make([]api.ObserveOption, 0, len(g.opts)+len(lbls))
	opts = append(opts, g.opts...)
	ng := &MetricGauge{
		gauge:       g.gauge,
		initializer: g.initializer,
		curValue:    atomic.Value{},
		derived:     make(map[string]*MetricGauge),
	}
	ng.curValue.Store(float64(0))
	ng.opts = append(opts, api.WithAttributes(lbls...))
	g.derived[key] = ng
	return ng
}

func (g *MetricGauge) Record(value float64) {
	if g.gauge == nil {
		g.gauge = g.initializer()
	}
	g.Do(func() {
		otel.GetMeterProvider().Meter(util.MetricPrefix).RegisterCallback(
			func(ctx context.Context, o api.Observer) error {
				o.ObserveFloat64(g.gauge, g.curValue.Load().(float64), g.opts...)
				return nil
			}, g.gauge)
	})
	g.curValue.Store(value)
}

func buildKeyFromLabels(lbls []attribute.KeyValue) string {
	sort.SliceStable(lbls, func(i, j int) bool {
		if lbls[i].Key == lbls[j].Key {
			return lbls[i].Value.AsString() < lbls[j].Value.AsString()
		}
		return lbls[i].Key < lbls[j].Key
	})
	var b strings.Builder
	for _, lbl := range lbls {
		b.WriteString(string(lbl.Key))
		b.WriteString(lbl.Value.AsString())
		b.WriteString("/")
	}
	return b.String()
}
