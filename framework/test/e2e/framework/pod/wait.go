package pod

import (
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/util/podutils"
	e2elog "slime.io/slime/framework/test/e2e/framework/log"
)

const (
	// poll is how often to poll pods, nodes and claims.
	poll = 2 * time.Second
	// podStartTimeout is how long to wait for the pod to be started.
	podStartTimeout = 5 * time.Minute
)

// WaitForPodRunningInNamespace waits default amount of time (podStartTimeout) for the specified pod to become running.
// Returns an error if timeout occurs first, or pod goes in to failed state.
func WaitForPodRunningInNamespace(c clientset.Interface, pod *v1.Pod) error {
	if pod.Status.Phase == v1.PodRunning {
		return nil
	}
	return WaitTimeoutForPodRunningInNamespace(c, pod.Name, pod.Namespace, podStartTimeout)
}

// WaitTimeoutForPodRunningInNamespace waits the given timeout duration for the specified pod to become running.
func WaitTimeoutForPodRunningInNamespace(c clientset.Interface, podName, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(poll, timeout, podRunning(c, podName, namespace))
}

// WaitTimeoutForPodReadyInNamespace waits the given timeout diration for the
// specified pod to be ready and running.
func WaitTimeoutForPodReadyInNamespace(c clientset.Interface, podName, namespace string, timeout time.Duration) error {
	return wait.PollImmediate(poll, timeout, podRunningAndReady(c, podName, namespace))
}

func podRunningAndReady(c clientset.Interface, podName, namespace string) wait.ConditionFunc {
	return func() (bool, error) {
		pod, err := c.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		switch pod.Status.Phase {
		case v1.PodFailed, v1.PodSucceeded:
			e2elog.Logf("The status of Pod %s is %s which is unexpected", podName, pod.Status.Phase)
			return false, errPodCompleted
		case v1.PodRunning:
			e2elog.Logf("The status of Pod %s is %s (Ready = %v)", podName, pod.Status.Phase, podutils.IsPodReady(pod))
			return podutils.IsPodReady(pod), nil
		}
		e2elog.Logf("The status of Pod %s is %s, waiting for it to be Running (with Ready = true)", podName, pod.Status.Phase)
		return false, nil
	}
}
