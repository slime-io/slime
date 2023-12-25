package monitoring

import (
	"fmt"

	"istio.io/libistio/pkg/monitoring"
)

const (
	UnitSeconds      = "s"
	UnitMilliseconds = "ms"
)

var SubModulesCount = monitoring.NewGauge(
	"framework_submodules_count",
	"total number of submodules",
)

func NewGauge(module string, name string, help string, options ...monitoring.Options) monitoring.Metric {
	gauge := monitoring.NewGauge(fmt.Sprintf("%s_%s", module, name), help, options...)
	return gauge
}

func NewSum(module string, name string, help string, options ...monitoring.Options) monitoring.Metric {
	sum := monitoring.NewSum(fmt.Sprintf("%s_%s", module, name), help, options...)
	return sum
}

func NewDistribution(module string, name string, help string, bounds []float64, options ...monitoring.Options) monitoring.Metric {
	histogram := monitoring.NewDistribution(fmt.Sprintf("%s_%s", module, name), help, bounds, options...)
	return histogram
}

func MustCreateLabel(key string) monitoring.Label {
	return monitoring.CreateLabel(key)
}

func WithUnit(unit string) monitoring.Options {
	return monitoring.WithUnit(monitoring.Unit(unit))
}
