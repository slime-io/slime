/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	testutil "slime.io/slime/framework/test/util"
	lazyloadv1aplha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg           *rest.Config
	testEnv       *envtest.Environment
	k8sClient     *kubernetes.Clientset
	dynamicClient dynamic.Interface

	scheme = runtime.NewScheme()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "lazyload Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			// crd of plugin
			filepath.Join("..", "charts", "crds"),
			// crd of common
			"../../../../../../../testdata/common/crds",
		},
		ErrorIfCRDPathMissing: true,
		// In CI, we use the environment KUBEBUILDER_ASSETS to set the BinaryAssetsDirectory field.
		// For local testing, please configure environment variables or set this field.
		// BinaryAssetsDirectory: "../../../../../../../testdata/bin/k8s/{version}-{os}-{arch}",
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	// add k8s scheme
	err = k8sscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// add istio scheme
	err = networkingv1alpha3.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// add lazyload scheme
	err = lazyloadv1aplha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	dynamicClient, err = dynamic.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func loadYamlTestData[T any](receiver *T, path string) error {
	return testutil.LoadYamlTestData(receiver, path)
}

func loadYamlObjects(path string) ([]client.Object, error) {
	return testutil.LoadYamlObjects(scheme, path)
}
