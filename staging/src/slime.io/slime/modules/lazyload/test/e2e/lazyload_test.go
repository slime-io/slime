package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"slime.io/slime/framework/test/e2e/framework"
	e2epod "slime.io/slime/framework/test/e2e/framework/pod"
	"slime.io/slime/framework/test/e2e/framework/testfiles"
)

var _ = ginkgo.Describe("Slime e2e test", func() {
	f := framework.NewDefaultFramework("lazyload")
	f.SkipNamespaceCreation = true

	ginkgo.It("clean resource", func() {
		var cmd *exec.Cmd
		cmd = exec.Command("bash", "lazyload_test_clean.sh")
		_, err := cmd.Output()
		framework.ExpectNoError(err)
	})

	ginkgo.It("rev: strictRev=true lazyload works", func() {
		_, err := f.CreateNamespace(nsSlime, nil)
		framework.ExpectNoError(err)
		_, err = f.CreateNamespace(nsApps, map[string]string{istioRevKey: substituteValue("istioRevValue", istioRevValue)})
		framework.ExpectNoError(err)

		strictRev := true
		createSlimeBoot(f)
		createSlimeModuleLazyload(f, strictRev)
		createExampleApps(f)
		createServiceFence(f, strictRev)
		updateSidecar(f)
		verifyAccessLogs(f)
		deleteTestResource()
	})

	ginkgo.It("no-rev: strictRev=false lazyload works", func() {
		_, err := f.CreateNamespace(nsSlime, nil)
		framework.ExpectNoError(err)
		_, err = f.CreateNamespace(nsApps, map[string]string{istioRevKey: substituteValue("istioRevValue", istioRevValue)})
		framework.ExpectNoError(err)

		strictRev := false
		createSlimeBoot(f)
		createSlimeModuleLazyload(f, strictRev)
		createExampleApps(f)
		createServiceFence(f, strictRev)
		updateSidecar(f)
		verifyAccessLogs(f)
		deleteTestResource()
	})
})

func createSlimeBoot(f *framework.Framework) {
	cs := f.ClientSet

	// install slimeboot
	crdYaml := readFile(test, "init/crds.yaml")
	framework.RunKubectlOrDieInput("", crdYaml, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: "", Contents: crdYaml})
	}()
	deploySlimeBootYaml := readFile(test, "init/deployment_slime-boot.yaml")
	deploySlimeBootYaml = strings.ReplaceAll(deploySlimeBootYaml, "{{slimebootTag}}", substituteValue("slimeBootTag", slimebootTag))
	framework.RunKubectlOrDieInput(nsSlime, deploySlimeBootYaml, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsSlime, Contents: deploySlimeBootYaml})
	}()

	slimebootDeploymentInstalled := false

	for i := 0; i < 10; i++ {
		pods, err := cs.CoreV1().Pods(nsSlime).List(metav1.ListOptions{})
		framework.ExpectNoError(err)
		if len(pods.Items) == 0 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, pod := range pods.Items {
			err = e2epod.WaitTimeoutForPodReadyInNamespace(cs, pod.Name, nsSlime, framework.PodStartTimeout)
			framework.ExpectNoError(err)
			if strings.Contains(pod.Name, slimebootName) {
				slimebootDeploymentInstalled = true
			}
		}
		break
	}
	if !slimebootDeploymentInstalled {
		framework.Failf("deployment slime-boot installation failed\n")
	}
	ginkgo.By("deployment slimeboot installs successfully")
}

func createSlimeModuleLazyload(f *framework.Framework, strictRev bool) {
	cs := f.ClientSet

	slimebootLazyloadYaml := readFile(test, "samples/lazyload/slimeboot_lazyload.yaml")
	slimebootLazyloadYaml = strings.ReplaceAll(slimebootLazyloadYaml, "{{lazyloadTag}}", substituteValue("lazyloadTag", lazyloadTag))
	slimebootLazyloadYaml = strings.ReplaceAll(slimebootLazyloadYaml, "{{globalSidecarTag}}", substituteValue("globalSidecarTag", globalSidecarTag))
	slimebootLazyloadYaml = strings.ReplaceAll(slimebootLazyloadYaml, "{{globalSidecarPilotTag}}", substituteValue("globalSidecarPilotTag", globalSidecarPilotTag))
	slimebootLazyloadYaml = strings.ReplaceAll(slimebootLazyloadYaml, "{{istioRevValue}}", substituteValue("istioRevValue", istioRevValue))
	if strictRev {
		slimebootLazyloadYaml = strings.ReplaceAll(slimebootLazyloadYaml, "{{strictRev}}", "true")
	} else {
		slimebootLazyloadYaml = strings.ReplaceAll(slimebootLazyloadYaml, "{{strictRev}}", "false")
	}
	framework.RunKubectlOrDieInput(nsSlime, slimebootLazyloadYaml, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsSlime, Contents: slimebootLazyloadYaml})
	}()

	// check
	lazyloadDeploymentInstalled := false
	globalSidecarPilotInstalled := false
	globalSidecarInstalled := false

	for i := 0; i < 60; i++ {
		pods, err := cs.CoreV1().Pods(nsSlime).List(metav1.ListOptions{})
		framework.ExpectNoError(err)
		if len(pods.Items) != 3 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, pod := range pods.Items {
			err = e2epod.WaitTimeoutForPodReadyInNamespace(cs, pod.Name, nsSlime, framework.PodStartTimeout)
			framework.ExpectNoError(err)
			if strings.Contains(pod.Name, "lazyload") {
				lazyloadDeploymentInstalled = true
			}
			if strings.Contains(pod.Name, "global-sidecar-pilot") {
				globalSidecarPilotInstalled = true
			}
		}
		break
	}

	for i := 0; i < 60; i++ {
		pods, err := f.ClientSet.CoreV1().Pods(nsApps).List(metav1.ListOptions{})
		framework.ExpectNoError(err)
		if len(pods.Items) == 0 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, pod := range pods.Items {
			err = e2epod.WaitTimeoutForPodReadyInNamespace(cs, pod.Name, nsApps, framework.PodStartTimeout)
			framework.ExpectNoError(err)
			if strings.Contains(pod.Name, "global-sidecar") {
				globalSidecarInstalled = true
			}
		}
		break
	}

	if !lazyloadDeploymentInstalled {
		framework.Failf("deployment lazyload installation failed\n")
	}
	if !globalSidecarPilotInstalled {
		framework.Failf("global-sidecar-pilot installation failed\n")
	}
	if !globalSidecarInstalled {
		framework.Failf("global-sidecar installation failed\n")
	}
	ginkgo.By("slimemodule lazyload installs successfully")
}

func createExampleApps(f *framework.Framework) {
	cs := f.ClientSet

	exampleAppsYaml := readFile(test, "config/bookinfo.yaml")
	framework.RunKubectlOrDieInput(nsApps, exampleAppsYaml, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsApps, Contents: exampleAppsYaml})
	}()

	// check
	for i := 0; i < 60; i++ {
		pods, err := cs.CoreV1().Pods(nsApps).List(metav1.ListOptions{})
		framework.ExpectNoError(err)
		if len(pods.Items) != 6 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for _, pod := range pods.Items {
			err = e2epod.WaitTimeoutForPodReadyInNamespace(cs, pod.Name, nsApps, framework.PodStartTimeout)
			framework.ExpectNoError(err)
		}
		break
	}
	ginkgo.By("example apps install successfully")
}

func createServiceFence(f *framework.Framework, strictRev bool) {
	// create CR ServiceFence
	serviceFenceYaml := readFile(test, "samples/lazyload/servicefence_productpage.yaml")
	serviceFenceYaml = strings.ReplaceAll(serviceFenceYaml, "{{istioRevKey}}", substituteValue("istioRevKey", istioRevKey))
	if strictRev {
		serviceFenceYaml = strings.ReplaceAll(serviceFenceYaml, "{{istioRevValue}}", substituteValue("strictRev", istioRevValue))
	} else {
		serviceFenceYaml = strings.ReplaceAll(serviceFenceYaml, "{{istioRevValue}}", "")
	}
	framework.RunKubectlOrDieInput(nsApps, serviceFenceYaml, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsApps, Contents: serviceFenceYaml})
	}()

	// check
	svfGvr := schema.GroupVersionResource{
		Group:    svfGroup,
		Version:  svfVersion,
		Resource: svfResource,
	}

	svfCreated := false
	for i := 0; i < 60; i++ {
		svf, err := f.DynamicClient.Resource(svfGvr).Namespace(nsApps).Get(svfName, metav1.GetOptions{})
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if (strictRev && svf.GetLabels()[istioRevKey] != istioRevValue) || (!strictRev && svf.GetLabels()[istioRevKey] != "") {
			framework.Failf("The created servicefence has wrong istioRev label %s.\n", svf.GetLabels()[istioRevKey])
		}
		svfCreated = true
		break
	}
	if svfCreated != true {
		framework.Failf("Failed to create servicefence.\n")
	}

	sidecarGvr := schema.GroupVersionResource{
		Group:    sidecarGroup,
		Version:  sidecarVersion,
		Resource: sidecarResource,
	}

	sidecarCreated := false
	for i := 0; i < 60; i++ {
		sidecar, err := f.DynamicClient.Resource(sidecarGvr).Namespace(nsApps).Get(sidecarName, metav1.GetOptions{})
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if (strictRev && sidecar.GetLabels()[istioRevKey] != istioRevValue) || (!strictRev && sidecar.GetLabels()[istioRevKey] != "") {
			framework.Failf("The created sidecar has wrong istioRev label %s.\n", sidecar.GetLabels()[istioRevKey])
		}
		sidecarCreated = true
		break
	}
	if sidecarCreated != true {
		framework.Failf("Failed to create sidecar.\n")
	}

	ginkgo.By("serviceFence and Sidecar create successfully")
}

func updateSidecar(f *framework.Framework) {
	pods, err := f.ClientSet.CoreV1().Pods(nsApps).List(metav1.ListOptions{})
	framework.ExpectNoError(err)
ExecLoop:
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "ratings") {
			total, success := 0, 0
			for {
				total++
				_, _, err = f.ExecShellInPodWithFullOutput(pod.Name, nsApps, "curl \"productpage:9080/productpage\"")
				if err == nil {
					success++
					if success >= 30 {
						break ExecLoop
					}
				}
				if total < 120 {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				break ExecLoop
				// framework.ExpectNoError(err)
			}
		}
	}

	sidecarGvr := schema.GroupVersionResource{
		Group:    sidecarGroup,
		Version:  sidecarVersion,
		Resource: sidecarResource,
	}
	sidecarUpdated := false
VerifyLoop:
	for i := 0; i < 120; i++ {
		sidecar, err := f.DynamicClient.Resource(sidecarGvr).Namespace(nsApps).Get(sidecarName, metav1.GetOptions{})
		framework.ExpectNoError(err)
		egress, _, err := unstructured.NestedSlice(sidecar.Object, "spec", "egress")
		framework.ExpectNoError(err)
		hosts, _, err := unstructured.NestedStringSlice(egress[0].(map[string]interface{}), "hosts")
		framework.ExpectNoError(err)
		for _, host := range hosts {
			if strings.Contains(host, "details") || strings.Contains(host, "reviews") {
				sidecarUpdated = true
				break VerifyLoop
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !sidecarUpdated {
		framework.Failf("sidecar updated failed\n")
	}
	ginkgo.By("sidecar updated successfully")
}

func verifyAccessLogs(f *framework.Framework) {
	cs := f.ClientSet
	pods, err := cs.CoreV1().Pods(nsApps).List(metav1.ListOptions{})
	framework.ExpectNoError(err)
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "productpage") {
			times := 0
			for {
				logs, err := e2epod.GetPodLogs(cs, nsApps, pod.Name, "istio-proxy")
				framework.ExpectNoError(err)
				if strings.Contains(logs, "outbound|9080||details") || strings.Contains(logs, "outbound|9080||reviews") {
					break
				} else {
					times++
				}
				if times > 60 {
					framework.Failf("access log verified failed\n")
				} else {
					time.Sleep(500 * time.Millisecond)
				}
			}
			break
		}
	}
	ginkgo.By("access log verified successfully")
}

func substituteValue(value, defaultValue string) string {
	if os.Getenv(value) != "" {
		return os.Getenv(value)
	}
	return defaultValue
}

func readFile(test, file string) string {
	from := filepath.Join(test, file)
	data, err := testfiles.Read(from)
	if err != nil {
		framework.ExpectNoError(err, "failed to read file %s/%s", test, file)
	}
	return string(data)
}

func deleteTestResource() {
	for i := len(testResourceToDelete) - 1; i >= 0; i-- {
		cleanupKubectlInputs(testResourceToDelete[i].Namespace, testResourceToDelete[i].Contents)
		time.Sleep(500 * time.Millisecond)
	}
}

// Stops everything from filePath from namespace ns and checks if everything matching selectors from the given namespace is correctly stopped.
// Aware of the kubectl example files map.
func cleanupKubectlInputs(ns string, fileContents string, selectors ...string) {
	ginkgo.By("using delete to clean up resources")
	// support backward compatibility : file paths or raw json - since we are removing file path
	// dependencies from this test.
	framework.RunKubectlOrDieInput(ns, fileContents, "delete", "--grace-period=0", "--force", "-f", "-")
	// assertCleanup(ns, selectors...)
}
