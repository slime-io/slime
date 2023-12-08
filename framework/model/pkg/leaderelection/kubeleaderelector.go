package leaderelection

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

var _ LeaderElector = &KubeLeaderElector{}

// KubeLeaderElector implements LeaderElector based on client-go leaderelection.
// The old leader has a few seconds to quit tasks before switching to the new
// leader. Tasks that do not exit in time can lead to the fact that there are
// two leaders at the same time.
// Furthermore, this implementation does not guarantee that there is only one
// leader. More information can be obtained from:
//
//	https://github.com/kubernetes/client-go/blob/master/tools/leaderelection/leaderelection.go
//
// NOTE:
// KubeLeaderElector does not strictly implement the LeaderElector, and
// the behavior differences are as follows:
//  1. The onStartedLeading callbacks are packaged and called asynchronously,
//     and executed synchronously in order in the asynchronous logic.
//  2. The onStoppedLeading callbacks will be executed when exiting,
//     even without being a leader.
type KubeLeaderElector struct {
	startCbLock               sync.RWMutex
	onStartedLeadingCallbacks []func(context.Context)
	stopCbLock                sync.RWMutex
	onStoppedLeadingCallbacks []func()

	id   string
	lock resourcelock.Interface
	le   *leaderelection.LeaderElector
}

func NewKubeLeaderElector(lock resourcelock.Interface) *KubeLeaderElector {
	return &KubeLeaderElector{
		id:   lock.Identity(),
		lock: lock,
	}
}

func (k *KubeLeaderElector) AddOnStartedLeading(cb func(context.Context)) {
	k.startCbLock.Lock()
	k.onStartedLeadingCallbacks = append(k.onStartedLeadingCallbacks, cb)
	k.startCbLock.Unlock()
}

func (k *KubeLeaderElector) AddOnStoppedLeading(cb func()) {
	k.stopCbLock.Lock()
	k.onStoppedLeadingCallbacks = append(k.onStoppedLeadingCallbacks, cb)
	k.stopCbLock.Unlock()
}

func (k *KubeLeaderElector) Run(ctx context.Context) error {
	if k.le == nil {
		var err error
		k.le, err = leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
			Lock: k.lock,
			// from: https://github.com/kubernetes/component-base/blob/master/config/v1alpha1/defaults.go
			LeaseDuration:   15 * time.Second,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			ReleaseOnCancel: true,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					k.startCbLock.RLock()
					cbs := make([]func(context.Context), len(k.onStartedLeadingCallbacks))
					copy(cbs, k.onStartedLeadingCallbacks)
					k.startCbLock.RUnlock()
					for _, f := range cbs {
						f(ctx)
					}
				},
				OnStoppedLeading: func() {
					k.stopCbLock.RLock()
					cbs := make([]func(), len(k.onStoppedLeadingCallbacks))
					copy(cbs, k.onStoppedLeadingCallbacks)
					k.stopCbLock.RUnlock()
					for _, f := range cbs {
						f()
					}
				},
			},
		})
		if err != nil {
			return fmt.Errorf("kube elector %s create elector failed: %s ", k.id, err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		k.le.Run(ctx)
	}
}

// NewKubeResourceLock create a new kube resourcelock.
// Do not accept a finished client, we can customize some configurations
// based on the basic cfg to create a client that is more suitable for
// election scenarios.
func NewKubeResourceLock(config *rest.Config, namespace, name string) (resourcelock.Interface, error) {
	if config == nil {
		return nil, fmt.Errorf("can not create kube resourcelock with empty config")
	}
	cfg := rest.CopyConfig(config)
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init kube client with config %v failed: %s ", cfg, err)
	}
	workload, _ := os.Hostname()
	id := workload + "_" + uuid.New().String()
	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace,
		name,
		client.CoreV1(),
		client.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id,
		})
	if err != nil {
		return nil, fmt.Errorf("create kube resourcelock %s failed: %s ", id, err)
	}
	return &resourcelockWrapper{
		Interface: lock,
	}, nil
}

type resourcelockWrapper struct {
	sync.Mutex
	resourcelock.Interface
}

func (l *resourcelockWrapper) Get(ctx context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	l.Lock()
	defer l.Unlock()
	return l.Interface.Get(ctx)
}

func (l *resourcelockWrapper) Create(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	l.Lock()
	defer l.Unlock()
	if _, _, err := l.Interface.Get(ctx); err == nil {
		return l.Interface.Update(ctx, ler)
	}
	return l.Interface.Create(ctx, ler)
}

func (l *resourcelockWrapper) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	l.Lock()
	defer l.Unlock()
	return l.Interface.Update(ctx, ler)
}

func (l *resourcelockWrapper) RecordEvent(event string) {
	l.Lock()
	l.Interface.RecordEvent(event)
	l.Unlock()
}
