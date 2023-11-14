package monitoring

import (
	"fmt"
	"istio.io/pkg/monitoring"
)

var (
	SubModulesCount = monitoring.NewGauge(
		"framework_submodules_count",
		"total number of submodules",
	)
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

func MustCreateLabel(key string) monitoring.Label {
	return monitoring.MustCreateLabel(key)
}

func WithLabels(resourceName monitoring.Label) monitoring.Options {
	return monitoring.WithLabels(resourceName)
}
