package framework

import (
	"bytes"
	"io"
	"net/url"
	"strings"

	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecOptions passed to ExecWithOptions
type ExecOptions struct {
	Command       []string
	Namespace     string
	PodName       string
	ContainerName string
	Stdin         io.Reader
	CaptureStdout bool
	CaptureStderr bool
	// If false, whitespace in std{err,out} will be removed.
	PreserveWhitespace bool
}

// ExecShellInPod executes the specified command on the pod.
func (f *Framework) ExecShellInPod(podName string, ns string, cmd string) string {
	return f.execCommandInPod(podName, ns, "/bin/sh", "-c", cmd)
	// return f.execCommandInPod(podName, ns, cmd)
}

// ExecShellInPodWithFullOutput executes the specified command on the Pod and returns stdout, stderr and error.
func (f *Framework) ExecShellInPodWithFullOutput(podName, ns string, cmd string) (string, string, error) {
	return f.execCommandInPodWithFullOutput(podName, ns, "/bin/sh", "-c", cmd)
	// return f.execCommandInPodWithFullOutput(podName, ns, cmd)
}

func (f *Framework) execCommandInPod(podName, ns string, cmd ...string) string {
	pod, err := f.ClientSet.CoreV1().Pods(ns).Get(podName, metav1.GetOptions{})
	ExpectNoError(err, "failed to get pod %v", podName)
	gomega.Expect(pod.Spec.Containers).NotTo(gomega.BeEmpty())
	return f.ExecCommandInContainer(podName, pod.Spec.Containers[0].Name, ns, cmd...)
}

func (f *Framework) execCommandInPodWithFullOutput(podName, ns string, cmd ...string) (string, string, error) {
	pod, err := f.ClientSet.CoreV1().Pods(ns).Get(podName, metav1.GetOptions{})
	ExpectNoError(err, "failed to get pod %v", podName)
	gomega.Expect(pod.Spec.Containers).NotTo(gomega.BeEmpty())
	return f.ExecCommandInContainerWithFullOutput(podName, pod.Spec.Containers[0].Name, ns, cmd...)
}

// ExecCommandInContainer executes a command in the specified container.
func (f *Framework) ExecCommandInContainer(podName, containerName, ns string, cmd ...string) string {
	stdout, stderr, err := f.ExecCommandInContainerWithFullOutput(podName, containerName, ns, cmd...)
	Logf("Exec stderr: %q", stderr)
	ExpectNoError(err,
		"failed to execute command in pod %v, container %v: %v",
		podName, containerName, err)
	return stdout
}

// ExecCommandInContainerWithFullOutput executes a command in the
// specified container and return stdout, stderr and error
func (f *Framework) ExecCommandInContainerWithFullOutput(podName, containerName, ns string, cmd ...string) (string, string, error) {
	return f.ExecWithOptions(ExecOptions{
		Command:            cmd,
		Namespace:          ns,
		PodName:            podName,
		ContainerName:      containerName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})
}

// ExecWithOptions executes a command in the specified container,
// returning stdout, stderr and error. `options` allowed for
// additional parameters to be passed.
func (f *Framework) ExecWithOptions(options ExecOptions) (string, string, error) {
	Logf("ExecWithOptions %+v", options)

	config, err := LoadConfig()
	ExpectNoError(err, "failed to load restclient config")

	const tty = false

	req := f.ClientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(options.PodName).
		Namespace(options.Namespace).
		SubResource("exec").
		Param("container", options.ContainerName)
	req.VersionedParams(&v1.PodExecOptions{
		Container: options.ContainerName,
		Command:   options.Command,
		Stdin:     options.Stdin != nil,
		Stdout:    options.CaptureStdout,
		Stderr:    options.CaptureStderr,
		TTY:       tty,
	}, scheme.ParameterCodec)

	var stdout, stderr bytes.Buffer
	err = execute("POST", req.URL(), config, options.Stdin, &stdout, &stderr, tty)

	if options.PreserveWhitespace {
		return stdout.String(), stderr.String(), err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func execute(method string, url *url.URL, config *restclient.Config, stdin io.Reader, stdout, stderr io.Writer, tty bool) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}
	return exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    tty,
	})
}
