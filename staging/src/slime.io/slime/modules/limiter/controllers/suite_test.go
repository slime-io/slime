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
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/modules/limiter/api/config"
	limiterv1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	scheme    = runtime.NewScheme()
	codecs    = serializer.NewCodecFactory(scheme)

	slimeEnv bootstrap.Environment
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "limiter controller suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

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
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// add k8s scheme
	err = k8sscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// add istio scheme
	err = networkingv1alpha3.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// add limiter scheme
	err = limiterv1alpha2.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// init slime env
	// TODO: support reload
	slimeEnv.Config = &bootconfig.Config{
		Global: &bootconfig.Global{
			IstioRev:       "default",
			IstioNamespace: "istio-system",
			ConfigSources: []*bootconfig.ConfigSource{
				{Address: "k8s://"},
			},
			Misc: map[string]string{
				"seLabelSelectorKeys": "app",
			},
		},
	}
	slimeEnv.Stop = ctx.Done()
	slimeEnv.K8SClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(slimeEnv.K8SClient).NotTo(BeNil())
	slimeEnv.DynamicClient, err = dynamic.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(slimeEnv.DynamicClient).NotTo(BeNil())
	slimeEnv.ConfigController, err = bootstrap.NewConfigController(slimeEnv.Config.Global.ConfigSources, ctx.Done())
	Expect(err).NotTo(HaveOccurred())
	_, err = bootstrap.RunController(slimeEnv.ConfigController, slimeEnv.Config, cfg)
	Expect(err).NotTo(HaveOccurred())

	// create controller manager
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	// setup SmartLimiterReconciler
	// TODO: support reload
	limiterConfig := &config.Limiter{
		DisableAdaptive:              true,
		DisableInsertLocalRateLimit:  true,
		DisableInsertGlobalRateLimit: true,
		RlsConfigMap:                 &config.RlsConfigMap{},
	}
	pc, err := NewProducerConfig(slimeEnv, limiterConfig)
	Expect(err).ToNot(HaveOccurred())
	source := metric.NewSource(pc)
	smartLimiterReconciler := NewReconciler(
		ReconcilerWithCfg(limiterConfig),
		ReconcilerWithEnv(slimeEnv),
		ReconcilerWithProducerConfig(pc),
		ReconcilerWithSource(source),
	)
	smartLimiterReconciler.Client = k8sManager.GetClient()
	smartLimiterReconciler.Scheme = k8sManager.GetScheme()

	err = smartLimiterReconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		metric.NewProducer(pc, source)
		smartLimiterReconciler.WatchMetric(ctx)
	}()

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func loadYamlObjects(path string) ([]client.Object, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	items := bytes.Split(data, []byte("---\n"))
	objs := make([]client.Object, 0, len(items))
	for _, item := range items {
		item = bytes.TrimSpace(item)
		if len(item) == 0 {
			continue
		}
		obj, _, err := codecs.UniversalDeserializer().Decode(item, nil, nil)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj.(client.Object))
	}
	return objs, nil
}

func loadYamlTestData[T any](receiver *T, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, receiver); err != nil {
		return err
	}
	return nil
}
