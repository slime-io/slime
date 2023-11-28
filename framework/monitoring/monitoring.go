package monitoring

import (
	"fmt"

	"istio.io/pkg/monitoring"
)

const (
	UnitSeconds      = "s"
	UnitMilliseconds = "ms"
)

var SubModulesCount = monitoring.NewGauge(
	"framework_submodules_count",
	"total number of submodules",
)

func init() {
	monitoring.MustRegister(
		SubModulesCount,
	)
}

func NewGauge(module string, name string, help string, options ...monitoring.Options) monitoring.Metric {
	gauge := monitoring.NewGauge(fmt.Sprintf("%s_%s", module, name), help, options...)
	monitoring.MustRegister(gauge)
	return gauge
}

func NewSum(module string, name string, help string, options ...monitoring.Options) monitoring.Metric {
	sum := monitoring.NewSum(fmt.Sprintf("%s_%s", module, name), help, options...)
	monitoring.MustRegister(sum)
	return sum
}

func NewDistribution(module string, name string, help string, bounds []float64, options ...monitoring.Options) monitoring.Metric {
	histogram := monitoring.NewDistribution(fmt.Sprintf("%s_%s", module, name), help, bounds, options...)
	monitoring.MustRegister(histogram)
	return histogram
}

func MustCreateLabel(key string) monitoring.Label {
	return monitoring.MustCreateLabel(key)
}

func WithLabels(resourceName ...monitoring.Label) monitoring.Options {
	return monitoring.WithLabels(resourceName...)
}

func WithUnit(unit string) monitoring.Options {
	return monitoring.WithUnit(monitoring.Unit(unit))
}
