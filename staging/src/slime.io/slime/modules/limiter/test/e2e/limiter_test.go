package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"slime.io/slime/framework/test/e2e/framework"
	e2epod "slime.io/slime/framework/test/e2e/framework/pod"
	"slime.io/slime/framework/test/e2e/framework/testfiles"
)

var _ = ginkgo.Describe("SmartLimiter e2e test", func() {
	f := framework.NewDefaultFramework("limiter")
	f.SkipNamespaceCreation = true

	ginkgo.It("prepare slimeboot limiter ns and bookinfos", func() {
		_, err := f.CreateNamespace(nsSlime, nil)
		framework.ExpectNoError(err)
		_, err = f.CreateNamespace(nsApps, map[string]string{istioRevKey: substituteValue("istioRevValue", istioRevValue)})
		framework.ExpectNoError(err)
		createSlimeBoot(f)
		createSlimeModuleLimiter(f)
		createExampleApps(f)
	})

	// all actions
	ginkgo.It("all", func() {
		_, err := f.CreateNamespace(nsSlime, nil)
		framework.ExpectNoError(err)
		_, err = f.CreateNamespace(nsApps, map[string]string{istioRevKey: substituteValue("istioRevValue", istioRevValue), "istio-injection": "enabled"})
		framework.ExpectNoError(err)
		createSlimeBoot(f)
		createSlimeModuleLimiter(f)
		createExampleApps(f)
		createSmartLimiter(f)
	})

	//  just apply a smartlimiter
	ginkgo.It("create smartlimiter and envoyfilters", func() {
		createSmartLimiter(f)
	})

	// apply a smartlimiter and check whether the limiter take effect
	ginkgo.It("verify limiter", func() {
		createSmartLimiter(f)
		limiterTackEffect(f)
	})
})

func createSlimeBoot(f *framework.Framework) {
	crdYaml := readFile(test, "init/crds.yaml")
	framework.RunKubectlOrDieInput("", crdYaml, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: "", Contents: crdYaml})
	}()
	cs := f.ClientSet
	deploySlimeBootYaml := readFile(test, "init/deployment_slime-boot.yaml")
	deploySlimeBootYaml = strings.ReplaceAll(deploySlimeBootYaml, "{{slimebootTag}}", substituteValue("slimeBootTag", slimebootTag))
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsSlime, Contents: deploySlimeBootYaml})
	}()
	framework.RunKubectlOrDieInput(nsSlime, deploySlimeBootYaml, "create", "-f", "-")
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

func createSlimeModuleLimiter(f *framework.Framework) {
	cs := f.ClientSet
	slimebootLimitYaml := readFile(test, "samples/limiter/slimeboot_limiter.yaml")
	slimebootLimitYaml = strings.ReplaceAll(slimebootLimitYaml, "{{limitTag}}", substituteValue("limitTag", limitTag))
	framework.RunKubectlOrDieInput(nsSlime, slimebootLimitYaml, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsSlime, Contents: slimebootLimitYaml})
	}()
	limitDeploymentInstalled := false

	for i := 0; i < 60; i++ {
		pods, err := cs.CoreV1().Pods(nsSlime).List(metav1.ListOptions{})
		framework.ExpectNoError(err)
		if len(pods.Items) <= 1 {
			time.Sleep(1000 * time.Millisecond)
			continue
		}
		for _, pod := range pods.Items {
			err = e2epod.WaitTimeoutForPodReadyInNamespace(cs, pod.Name, nsSlime, framework.PodStartTimeout)
			framework.ExpectNoError(err)
			if strings.Contains(pod.Name, "limiter") {
				limitDeploymentInstalled = true
				break
			}
		}
	}
	if !limitDeploymentInstalled {
		framework.Failf("deployment limiter installation failed\n")
	}
	ginkgo.By("slimemodule limit installs successfully")
}

func createExampleApps(f *framework.Framework) {
	cs := f.ClientSet

	exampleAppsYaml := readFile(test, "config/bookinfo.yaml")
	framework.RunKubectlOrDieInput(nsApps, exampleAppsYaml, "create", "-f", "-")
	//defer func() {
	//	testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsApps, Contents: exampleAppsYaml})
	//}()

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

func createSmartLimiter(f *framework.Framework) {
	smartLimiter := readFile(test, "samples/limiter/productpage_smartlimiter.yaml")
	framework.RunKubectlOrDieInput(nsApps, smartLimiter, "create", "-f", "-")
	defer func() {
		testResourceToDelete = append(testResourceToDelete, &TestResource{Namespace: nsApps, Contents: smartLimiter})
	}()

	smartLimiterGVR := schema.GroupVersionResource{
		Group:    slGroup,
		Version:  slVersion,
		Resource: slResource,
	}
	created := false
	for i := 0; i < 60; i++ {
		_, err := f.DynamicClient.Resource(smartLimiterGVR).Namespace(nsApps).Get("productpage", metav1.GetOptions{})
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		created = true
		break
	}
	if created != true {
		framework.Failf("Failed to create smartLimiter.\n")
	}

	created = false
	envoyFilterGVR := schema.GroupVersionResource{
		Group:    efGroup,
		Version:  efVersion,
		Resource: efResource,
	}
	for i := 0; i < 30; i++ {
		_, err := f.DynamicClient.Resource(envoyFilterGVR).Namespace(nsApps).Get("productpage.temp.ratelimit", metav1.GetOptions{})
		if err != nil {
			time.Sleep(1000 * time.Millisecond)
			continue
		}
		created = true
		break
	}
	if created != true {
		framework.Failf("Failed to create envoyFilter.\n")
	}
	ginkgo.By("smartLimiter and envoyFilter create successfully")
}

// curl -I http://productpage:9080/productpage
// curl -I http://reviews:9080/
func limiterTackEffect(f *framework.Framework) {
	time.Sleep(20 * time.Second)
	pods, err := f.ClientSet.CoreV1().Pods(nsApps).List(metav1.ListOptions{})
	framework.ExpectNoError(err)
	for _, pod := range pods.Items {
		if strings.Contains(pod.Name, "ratings") {
			for i := 1; i <= 10; i++ {
				output, _, err := f.ExecCommandInContainerWithFullOutput(pod.Name, "ratings", nsApps, "curl", "-I", "http://productpage:9080/productpage")
				framework.ExpectNoError(err)
				if i < 5 {
					if !strings.Contains(output, "200") {
						framework.Failf("servers productpage not found\n")
					}
				} else if !strings.Contains(output, "429") {
					framework.Failf("the smartLimiter action 4/min not take effect .\n")
				}
				time.Sleep(2 * time.Second)
			}
		}
	}
	ginkgo.By("smartLimiter action 4/min take effect")
}

func deleteTestResource() {
	for i := len(testResourceToDelete) - 1; i >= 0; i-- {
		cleanupKubectlInputs(testResourceToDelete[i].Namespace, testResourceToDelete[i].Contents)
		time.Sleep(500 * time.Millisecond)
	}
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

// Stops everything from filePath from namespace ns and checks if everything matching selectors from the given namespace is correctly stopped.
// Aware of the kubectl example files map.
func cleanupKubectlInputs(ns string, fileContents string, selectors ...string) {
	ginkgo.By("using delete to clean up resources")
	// support backward compatibility : file paths or raw json - since we are removing file path
	// dependencies from this test.
	framework.RunKubectlOrDieInput(ns, fileContents, "delete", "--grace-period=0", "--force", "-f", "-")
	// assertCleanup(ns, selectors...)
}
