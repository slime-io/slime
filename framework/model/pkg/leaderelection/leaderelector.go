package leaderelection

import (
	"context"
	"sync"
)

// LeaderCallbacks is used to register callback functions
type LeaderCallbacks interface {
	// AddOnStartedLeading add a callback that needs to be executed after
	// becoming a leader. The callback function needs to be non-blocking,
	// and the registered callbacks will be executed serially.
	// `context.Context` is used to help control the life cycle of resident
	// tasks that may be created by the callback. After stopping as a leader,
	// the `Context` will be closed. The callback should use the `Context`
	// reasonably to achieve the election effect. In particular, resident tasks
	// that need to be run by a single instance must exit when the `Context`
	// is closed.
	AddOnStartedLeading(func(context.Context))
	// AddOnStoppedLeading add a callback that needs to be executed after
	// stopping as a leader. The callback function needs to be non-blocking,
	// and the registered callbacks will be executed serially.
	AddOnStoppedLeading(func())
}

// LeaderElector supports starting tasks that need to keep a single instance
// running, and can switch workloads when the current workload is abnormal.
type LeaderElector interface {
	LeaderCallbacks
	// Run start the leader election.
	// The leader election is a loop until the `Context` is closed. The single
	// loop behavior is as follows:
	//   1. First try to become the leader until it becomes the leader or exits
	//      because the `Context` is closed;
	//   2. After becoming a leader, execute all OnStartedLeading callbacks
	//      synchronously in order;
	//   3. When switching from leader to candidate, the tasks derived from the
	//      OnStartedLeading callbacks can be notified asynchronously, and the
	//      OnStoppedLeading callbacks can be executed synchronously in order;
	//   4. When serving as a leader, if the `Context` is closed, follow `3`.
	Run(context.Context) error
}

var _ LeaderElector = &AlwaysLeader{}

type AlwaysLeader struct {
	startCbLock               sync.RWMutex
	onStartedLeadingCallbacks []func(context.Context)
	stopCbLock                sync.RWMutex
	onStoppedLeadingCallbacks []func()
}

func NewAlwaysLeader() *AlwaysLeader {
	return &AlwaysLeader{}
}

func (al *AlwaysLeader) AddOnStartedLeading(cb func(context.Context)) {
	al.startCbLock.Lock()
	al.onStartedLeadingCallbacks = append(al.onStartedLeadingCallbacks, cb)
	al.startCbLock.Unlock()
}

func (al *AlwaysLeader) AddOnStoppedLeading(cb func()) {
	al.stopCbLock.Lock()
	al.onStoppedLeadingCallbacks = append(al.onStoppedLeadingCallbacks, cb)
	al.stopCbLock.Unlock()
}

func (al *AlwaysLeader) Run(ctx context.Context) error {
	defer func() {
		al.stopCbLock.RLock()
		cbs := make([]func(), len(al.onStoppedLeadingCallbacks))
		copy(cbs, al.onStoppedLeadingCallbacks)
		al.stopCbLock.Unlock()
		for _, f := range cbs {
			f()
		}
	}()
	al.startCbLock.RLock()
	cbs := make([]func(context.Context), len(al.onStartedLeadingCallbacks))
	copy(cbs, al.onStartedLeadingCallbacks)
	al.startCbLock.RUnlock()
	for _, f := range cbs {
		f(ctx)
	}
	<-ctx.Done()
	return nil
}
