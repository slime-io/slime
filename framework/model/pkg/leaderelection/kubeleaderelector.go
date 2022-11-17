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
//   1. The onStartedLeading callbacks are packaged and called asynchronously,
//      and executed synchronously in order in the asynchronous logic.
//   2. The onStoppedLeading callbacks will be executed when exiting,
//      even without being a leader.
type KubeLeaderElector struct {
	startCbLock               sync.RWMutex
	onStartedLeadingCallbacks []func(context.Context)
	stopCbLock                sync.RWMutex
	onStopLeadingCallbacks    []func()

	id        string
	name      string
	namespace string

	// config is the rest.config used to talk to the apiserver.
	config *rest.Config

	le *leaderelection.LeaderElector
}

func NewKubeLeaderElector(config *rest.Config, namespace, name string) *KubeLeaderElector {
	workload, _ := os.Hostname()
	id := workload + "_" + uuid.New().String()
	return &KubeLeaderElector{
		id:        id,
		namespace: namespace,
		name:      name,
		config:    config,
	}
}

func (k *KubeLeaderElector) AddOnStartedLeading(cb func(context.Context)) {
	k.startCbLock.Lock()
	k.onStartedLeadingCallbacks = append(k.onStartedLeadingCallbacks, cb)
	k.startCbLock.Unlock()
}

func (k *KubeLeaderElector) AddOnStoppedLeading(cb func()) {
	k.stopCbLock.Lock()
	k.onStopLeadingCallbacks = append(k.onStopLeadingCallbacks, cb)
	k.stopCbLock.Unlock()
}

func (k *KubeLeaderElector) Run(ctx context.Context) error {
	if k.le == nil {
		if k.config == nil {
			// try in-cluster config
			config, err := rest.InClusterConfig()
			if err != nil {
				return fmt.Errorf("kube elector %s is without custom config, try to get in-cluster config failed: %s", k.id, err)
			}
			k.config = config
		}

		client, err := kubernetes.NewForConfig(k.config)
		if err != nil {
			return fmt.Errorf("kube elector %s init kube client failed: %s ", k.id, err)
		}

		lock, err := resourcelock.New(
			resourcelock.ConfigMapsLeasesResourceLock,
			k.namespace,
			k.name,
			client.CoreV1(),
			client.CoordinationV1(),
			resourcelock.ResourceLockConfig{
				Identity: k.id,
			})
		if err != nil {
			return fmt.Errorf("kube elector %s create resourcelock failed: %s ", k.id, err)
		}
		k.le, err = leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
			Lock:            lock,
			LeaseDuration:   15 * time.Second,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			ReleaseOnCancel: true,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					k.startCbLock.RLock()
					for _, f := range k.onStartedLeadingCallbacks {
						f(ctx)
					}
					k.startCbLock.Unlock()
				},
				OnStoppedLeading: func() {
					k.stopCbLock.RLock()
					for _, f := range k.onStopLeadingCallbacks {
						f()
					}
					k.stopCbLock.Unlock()
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
