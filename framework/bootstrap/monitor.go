package bootstrap

import (
	log "github.com/sirupsen/logrus"

	"slime.io/slime/framework/bootstrap/resource"
)

const (
	// BufferSize specifies the buffer size of event channel
	BufferSize = 100
)

type Handler func(resource.Config, resource.Config, Event)

// Monitor provides methods of manipulating changes in the config store
type Monitor interface {
	AppendEventHandler(resource.GroupVersionKind, Handler)
	Run(<-chan struct{})
	ScheduleProcessEvent(ConfigEvent)
}

type monitor struct {
	store    ConfigStore
	handlers map[resource.GroupVersionKind][]Handler
	eventCh  chan ConfigEvent
}

// NewMonitor returns new Monitor implementation with a default event buffer size.
func newMonitor(store ConfigStore) Monitor {
	return newBufferedMonitor(store, BufferSize)
}

// NewBufferedMonitor returns new Monitor implementation with the specified event buffer size
func newBufferedMonitor(store ConfigStore, bufferSize int) Monitor {
	handlers := make(map[resource.GroupVersionKind][]Handler)

	for _, s := range store.Schemas().All() {
		handlers[s.GroupVersionKind()] = make([]Handler, 0)
	}

	return &monitor{
		store:    store,
		handlers: handlers,
		eventCh:  make(chan ConfigEvent, bufferSize),
	}
}

func (m *monitor) AppendEventHandler(typ resource.GroupVersionKind, h Handler) {
	m.handlers[typ] = append(m.handlers[typ], h)
}

func (m *monitor) ScheduleProcessEvent(configEvent ConfigEvent) {
	m.eventCh <- configEvent
}

func (m *monitor) processConfigEvent(ce ConfigEvent) {
	if _, exists := m.handlers[ce.config.GroupVersionKind]; !exists {
		log.Warnf("gvk %s does not have handler", ce.config.GroupVersionKind)
		return
	}
	m.applyHandlers(ce.old, ce.config, ce.event)
}

func (m *monitor) applyHandlers(old resource.Config, config resource.Config, e Event) {
	for _, f := range m.handlers[config.GroupVersionKind] {
		f(old, config, e)
	}
}

func (m *monitor) Run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case ce, ok := <-m.eventCh:
			if ok {
				m.processConfigEvent(ce)
			}
		}
	}
}

// ConfigEvent defines the event to be processed
type ConfigEvent struct {
	config resource.Config
	old    resource.Config
	event  Event
}

// Event represents a registry update event
type Event int

const (
	EventAdd Event = iota
	EventUpdate
	EventDelete
)

func (event Event) String() string {
	out := "unknown"
	switch event {
	case EventAdd:
		out = "add"
	case EventUpdate:
		out = "update"
	case EventDelete:
		out = "delete"
	}
	return out
}
