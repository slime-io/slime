package monitoring

import (
	"time"

	"slime.io/slime/framework/monitoring"
	"slime.io/slime/modules/meshregistry/model"
)

var (
	// souceLabel is the label for the source of the service entry.
	souceLabel = monitoring.MustCreateLabel("source")
	// statusLabel is the label for the status of the event.
	statusLabel = monitoring.MustCreateLabel("status")
)

var (

	// enabledSource is the number of enabled sources.
	enabledSource = monitoring.NewGauge(
		model.ModuleName,
		"enabled_source",
		"Number of enabled sources.",
	)

	// readyTime is the time spent on ready in seconds.
	readyTime = monitoring.NewGauge(
		model.ModuleName,
		"ready_time",
		"Time spent on ready in seconds",
	)

	// mcpPushCount is the number of mcp push.
	mcpPushCount = monitoring.NewSum(
		model.ModuleName,
		"mcp_push_count",
		"Number of mcp push.",
	)
)

// RecordEnabledSource records the number of enabled sources.
func RecordEnabledSource(count int) {
	enabledSource.Record(float64(count))
}

// RecordReady records the time spent on ready.
func RecordReady(source string, t0, t1 time.Time) {
	readyTime.With(souceLabel.Value(source)).Record(float64(t1.Sub(t0).Seconds()))
}

// RecordMcpPush records the number of mcp push.
func RecordMcpPush() {
	mcpPushCount.Increment()
}
