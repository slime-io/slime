package monitoring

import (
	"fmt"
	"time"

	"istio.io/libistio/pkg/config/event"

	"slime.io/slime/framework/bootstrap/collections"
	"slime.io/slime/framework/monitoring"
	"slime.io/slime/modules/meshregistry/model"
)

var (
	// configTypeLabel is the label for the type of the config.
	configTypeLabel = monitoring.MustCreateLabel("config_type")

	servceEntryType = collections.IstioNetworkingV1Alpha3Serviceentries.String()
	sidecarType     = collections.IstioNetworkingV1Alpha3Sidecars.String()
)

var (
	// pollingTime is the time spent on polling in seconds.
	pollingTime = monitoring.NewDistribution(
		model.ModuleName,
		"polling_time",
		"Time spent on polling in seconds",
		[]float64{.5, 1, 3, 5, 8, 10, 20, 30, 60, 90, 120},
		monitoring.WithUnit(monitoring.UnitSeconds),
	)
	// pollingsCount is the number of pollings by source.
	pollingsCount = monitoring.NewSum(
		model.ModuleName,
		"pollings_count",
		"Number of pollings by source.",
	)

	// serviceEntryTotal is the number of service entries in the mesh registry by source.
	serviceEntryTotal = monitoring.NewSum(
		model.ModuleName,
		"service_entry_total",
		"Number of service entries in the mesh registry by source.",
	)
	// serviceEntryCreation is the number of service entries created by source.
	serviceEntryCreation = monitoring.NewSum(
		model.ModuleName,
		"service_entry_creation",
		"Number of service entries created by source.",
	)
	// serviceEntryDeletion is the number of service entries deleted by source.
	serviceEntryDeletion = monitoring.NewSum(
		model.ModuleName,
		"service_entry_deletion",
		"Number of service entries deleted by source,"+
			" or the endpoints of service entry are cleared.",
	)
	// serviceEntryUpdate is the number of service entries updated by source.
	serviceEntryUpdate = monitoring.NewSum(
		model.ModuleName,
		"service_entry_update",
		"Number of service entries updated by source.",
	)

	// sidecarTotal is the number of sidecars in the mesh registry by source.
	sidecarTotal = monitoring.NewSum(
		model.ModuleName,
		"sidecar_total",
		"Number of sidecars in the mesh registry by source.",
	)

	// sidecarCreation is the number of sidecars created by source.
	sidecarCreation = monitoring.NewSum(
		model.ModuleName,
		"sidecar_creation",
		"Number of sidecars created by source.",
	)
	// sidecarDeletion is the number of sidecars deleted by source.
	sidecarDeletion = monitoring.NewSum(
		model.ModuleName,
		"sidecar_deletion",
		"Number of sidecars deleted by source.",
	)
	// sidecarUpdate is the number of sidecars updated by source.
	sidecarUpdate = monitoring.NewSum(
		model.ModuleName,
		"sidecar_update",
		"Number of sidecars updated by source.",
	)

	// sourceEventTypeLabel is the label for the type of the source event.
	sourceEventTypeLabel = monitoring.MustCreateLabel("event_type")
	// sourceEventCount is the number of events by source.
	sourceEventCount = monitoring.NewSum(
		model.ModuleName,
		"source_event_count",
		"config event count by source, with event type and config type, and status.",
	)

	// sourceClientRequestCount is the number of client requests by source, with status.
	sourceClientRequestCount = monitoring.NewSum(
		model.ModuleName,
		"source_client_request_count",
		"client request count by source, with status.",
	)
)

// RecordPolling records the time spent on each polling and increment the number of pollings by source.
func RecordPolling(source string, t0, t1 time.Time, success bool) {
	pollingTime.With(
		souceLabel.Value(source),
		statusLabel.Value(fmt.Sprintf("%t", success)),
	).Record(float64(t1.Sub(t0).Seconds()))
	pollingsCount.With(
		souceLabel.Value(source),
		statusLabel.Value(fmt.Sprintf("%t", success)),
	).Increment()
}

// RecordServiceEntryCreation records the number of service entries created by source.
func RecordServiceEntryCreation(source string, buildEvent bool) {
	serviceEntryCreation.With(souceLabel.Value(source)).Increment()
	serviceEntryTotal.With(souceLabel.Value(source)).Increment()
	sourceEventCount.With(
		souceLabel.Value(source),
		configTypeLabel.Value(servceEntryType),
		sourceEventTypeLabel.Value(event.Added.String()),
		statusLabel.Value(fmt.Sprintf("%t", buildEvent)),
	).Increment()
}

// RecordServiceEntryDeletion records the number of service entries deleted by source.
func RecordServiceEntryDeletion(source string, delete, buildEvent bool) {
	serviceEntryDeletion.With(souceLabel.Value(source)).Increment()
	eventType := event.Updated
	if delete {
		// todo: decrement
		// serviceEntryTotal.With(souceLabel.Value(source)).Decrement()
		eventType = event.Deleted
	}
	sourceEventCount.With(
		souceLabel.Value(source),
		configTypeLabel.Value(servceEntryType),
		sourceEventTypeLabel.Value(eventType.String()),
		statusLabel.Value(fmt.Sprintf("%t", buildEvent)),
	).Increment()
}

// RecordServiceEntryUpdate records the number of service entries updated by source.
func RecordServiceEntryUpdate(source string, buildEvent bool) {
	serviceEntryUpdate.With(souceLabel.Value(source)).Increment()
	sourceEventCount.With(
		souceLabel.Value(source),
		configTypeLabel.Value(servceEntryType),
		sourceEventTypeLabel.Value(event.Updated.String()),
		statusLabel.Value(fmt.Sprintf("%t", buildEvent)),
	).Increment()
}

// RecordSidecarCreation records the number of sidecars created by source.
func RecordSidecarCreation(source string) {
	sidecarCreation.With(souceLabel.Value(source)).Increment()
	sidecarTotal.With(souceLabel.Value(source)).Increment()
	sourceEventCount.With(
		souceLabel.Value(source),
		configTypeLabel.Value(sidecarType),
		sourceEventTypeLabel.Value(event.Added.String()),
		statusLabel.Value("true"),
	).Increment()
}

// RecordSidecarDeletion records the number of sidecars deleted by source.
func RecordSidecarDeletion(source string) {
	sidecarDeletion.With(souceLabel.Value(source)).Increment()
	sidecarTotal.With(souceLabel.Value(source)).Decrement()
	sourceEventCount.With(
		souceLabel.Value(source),
		configTypeLabel.Value(sidecarType),
		sourceEventTypeLabel.Value(event.Deleted.String()),
		statusLabel.Value("true"),
	).Increment()
}

// RecordSidecarUpdate records the number of sidecars updated by source.
func RecordSidecarUpdate(source string) {
	sidecarUpdate.With(souceLabel.Value(source)).Increment()
	sourceEventCount.With(
		souceLabel.Value(source),
		configTypeLabel.Value(sidecarType),
		sourceEventTypeLabel.Value(event.Updated.String()),
		statusLabel.Value("true"),
	).Increment()
}

// RecordSourceClientRequest records the number of client requests by source.
func RecordSourceClientRequest(source string, success bool) {
	sourceClientRequestCount.With(
		souceLabel.Value(source),
		statusLabel.Value(fmt.Sprintf("%t", success)),
	).Increment()
}
