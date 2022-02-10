package trigger

import (
	"time"

	log "github.com/sirupsen/logrus"
)

type TickerTrigger struct {
	durations    []time.Duration
	durationsMap map[time.Duration]chan struct{}
	eventChan    chan TickerEvent
}

type TickerEvent struct {
	Duration time.Duration
}

type TickerTriggerConfig struct {
	Durations []time.Duration
	EventChan chan TickerEvent
}

func NewTickerTrigger(config TickerTriggerConfig) *TickerTrigger {
	return &TickerTrigger{
		durations:    config.Durations,
		eventChan:    config.EventChan,
		durationsMap: make(map[time.Duration]chan struct{}),
	}
}

func (t *TickerTrigger) Start() {
	log := log.WithField("reporter", "TickerTrigger").WithField("function", "Start")

	for _, duration := range t.durations {
		t.durationsMap[duration] = make(chan struct{})
		log.Infof("add timer %s to metric trigger", duration.String())
	}

	for duration, channel := range t.durationsMap {
		go func(du time.Duration, ch chan struct{}) {
			ticker := time.NewTicker(du)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						log.Debugf("stop a timer")
						return
					}
				case <-ticker.C:
					log.Debugf("got timer event: duration %v", du)
					event := TickerEvent{
						Duration: du,
					}
					t.eventChan <- event
					// log.Debugf("sent timer event to controller: duration %s", timer.String())
				}
			}
		}(duration, channel)
	}
}

func (t *TickerTrigger) Stop() {
	for _, ch := range t.durationsMap {
		close(ch)
	}
}

func (t *TickerTrigger) EventChan() <-chan TickerEvent {
	return t.eventChan
}
