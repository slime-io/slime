/*
* @Author: yangdihang
* @Date: 2021/1/7
 */

package e2e

import (
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	commonutils "k8s.io/kubernetes/test/e2e/common"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

func readFile(test, file string) string {
	from := filepath.Join(test, file)
	return commonutils.SubstituteImageName(string(testfiles.ReadOrDie(from)))
}

var _ = framework.KubeDescribe("[Slow]", func() {
	f := framework.NewDefaultFramework("slime-boot")

	framework.KubeDescribe("Install", func() {
		ginkgo.It("install slime", func() {
			test := "testdata/install"
			// 读取文件，Go Template形式，并解析为YAML资源清单
			crdYaml := readFile(test, "crds.yaml")
			slimeBootYaml := readFile(test, "slime-boot-install.yaml")

			// 创建slime-boot
			_, err := framework.CreateTestingNS("mesh-operator", f.ClientSet, nil)
			framework.ExpectNoError(err)

			framework.RunKubectlOrDieInput(crdYaml, "create", "-f", "-")
			framework.RunKubectlOrDieInput(slimeBootYaml, "create", "-f", "-")
			pods, err := f.ClientSet.CoreV1().Pods("mesh-operator").List(metav1.ListOptions{})
			framework.ExpectNoError(err)
			if len(pods.Items) == 0 {
				framework.Failf("slime-boot installation failed\n")
			}
			for _, pod := range pods.Items {
				err = e2epod.WaitForPodRunningInNamespace(f.ClientSet, &pod)
				framework.ExpectNoError(err)
			}

			ginkgo.By("slime boot install successfully")

			istioCrdYaml := readFile(test, "istio-crds.yaml")
			slimeBootConfigYaml := readFile(test, "slime-boot-install.yaml")

			framework.RunKubectlOrDieInput(istioCrdYaml, "create", "-f", "-")
			framework.RunKubectlOrDieInput(slimeBootConfigYaml, "create", "-f", "-")

			globalSidecarInstalled := false
			qzPilotInstalled := false
			reportDependencyInstalled := false

			pods, err = f.ClientSet.CoreV1().Pods("mesh-operator").List(metav1.ListOptions{})
			framework.ExpectNoError(err)
			for _, pod := range pods.Items {
				err = e2epod.WaitForPodRunningInNamespace(f.ClientSet, &pod)
				framework.ExpectNoError(err)
				if strings.Contains(pod.Name, "global-sidecar") {
					globalSidecarInstalled = true
				}
				if strings.Contains(pod.Name, "pilot") {
					qzPilotInstalled = true
				}
				if strings.Contains(pod.Name, "telemetry") {
					reportDependencyInstalled = true
				}
			}
			if !globalSidecarInstalled {
				framework.Failf("global-sidecar installation failed\n")
			}

			if !qzPilotInstalled {
				framework.Failf("pilot installation failed\n")
			}

			if !reportDependencyInstalled {
				framework.Failf("report-dependency installation failed\n")
			}
		})
	})
})
