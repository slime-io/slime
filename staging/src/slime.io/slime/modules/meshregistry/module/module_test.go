package module

import (
	"context"
	"os"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/module"
	meshregv1alpha1 "slime.io/slime/modules/meshregistry/api/v1alpha1"
	meshregbootstrap "slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/features"
)

var _ = Describe("DynamicConfigController", func() {
	podNamespaceBackup := PodNamespace
	dynCmBackup := features.DynamicConfigMap
	dynCrBackup := features.WatchingRegistrySource
	gotCh := make(chan *meshregbootstrap.RegistryArgs, 1)
	m := Module{
		dynConfigHandlers: []func(*meshregbootstrap.RegistryArgs){
			func(args *meshregbootstrap.RegistryArgs) {
				gotCh <- args
			},
		},
	}
	var opts module.ModuleOptions
	var ctx context.Context
	var cancel context.CancelFunc

	Describe("configuration Configurator", func() {
		cmGvr := corev1.SchemeGroupVersion.WithResource(string(corev1.ResourceConfigMaps))
		setFullConfigurator := func(ns, name string) {
			PodNamespace = ns
			features.DynamicConfigMap = name
		}
		crGvr := meshregv1alpha1.RegistrySourcesResource
		setPatchConfigurator := func(ns, name string) {
			PodNamespace = ns
			features.WatchingRegistrySource = name
		}

		BeforeEach(func() {
			opts = module.ModuleOptions{
				Env: bootstrap.Environment{
					K8SClient:     k8sClient,
					DynamicClient: dynamicClient,
					Config: &bootconfig.Config{
						Name: "meshregistry",
					},
				},
			}
			ctx, cancel = context.WithCancel(context.Background())
			opts.Env.Stop = ctx.Done()
		})
		AfterEach(func() {
			PodNamespace = podNamespaceBackup
			features.DynamicConfigMap = dynCrBackup
			features.WatchingRegistrySource = dynCmBackup
			cancel()
			ctx, cancel = nil, nil
		})

		// Paths for configurations and expectations:
		// - static: YAML-formatted bootconfig.Config.
		// - dynamic: YAML-formatted ConfigMap, store the new YAML-formatted bootconfig.Config in `cfg_meshregistry`
		// - expect: YAML-formatted bootstrap.RegistryArgs for comparison.
		DescribeTable("start with dynamic config",
			func(static, dynamic, expect string, gvr schema.GroupVersionResource, setF func(ns, name string)) {
				// load static config
				staticRegArgs, err := loadBootstrapConfig(static, false)
				Expect(err).ToNot(HaveOccurred())

				// load dynamic config
				var u unstructured.Unstructured
				err = loadYamlTestData(&u, dynamic)
				Expect(err).ToNot(HaveOccurred())

				// apply dynamic config
				_, err = dynamicClient.Resource(gvr).Namespace(u.GetNamespace()).Create(ctx, &u, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
				defer func() {
					_ = dynamicClient.Resource(gvr).Namespace(u.GetNamespace()).Delete(ctx, u.GetName(), metav1.DeleteOptions{})
				}()

				// load expected config
				expectedRegArgs, err := loadBootstrapConfig(expect, true)
				Expect(err).ToNot(HaveOccurred())

				// run dynamic config controller
				setF(u.GetNamespace(), u.GetName())
				got, err := m.prepareDynamicConfigController(opts, staticRegArgs)
				Expect(err).ToNot(HaveOccurred())
				Expect(cmp.Diff(got, expectedRegArgs)).Should(BeEmpty())
			},
			Entry("legacy gateway mode full configurator",
				"testdata/legacy-gw-mode.static.yaml", "testdata/legacy-gw-mode.dynamic.cm.yaml", "testdata/legacy-gw-mode.expect.cm.yaml", //nolint: lll
				cmGvr, setFullConfigurator),
			Entry("legacy gateway mode patch configurator",
				"testdata/legacy-gw-mode.static.yaml", "testdata/legacy-gw-mode.dynamic.cr.yaml", "testdata/legacy-gw-mode.expect.cr.yaml", //nolint: lll
				crGvr, setPatchConfigurator),
		)

		DescribeTable("start without dynamic config",
			func(static, dynamic, expect string, gvr schema.GroupVersionResource, setF func(ns, name string)) {
				// load static config
				staticRegArgs, err := loadBootstrapConfig(static, false)
				Expect(err).ToNot(HaveOccurred())

				// load dynamic config
				var u unstructured.Unstructured
				err = loadYamlTestData(&u, dynamic)
				Expect(err).ToNot(HaveOccurred())

				// run dynamic config controller
				setF(u.GetNamespace(), u.GetName())
				_, err = m.prepareDynamicConfigController(opts, staticRegArgs)
				Expect(err).ToNot(HaveOccurred())
				go m.reloadDynamicConfigTask(ctx)

				// apply dynamic config
				_, err = dynamicClient.Resource(gvr).Namespace(u.GetNamespace()).Create(ctx, &u, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
				defer func() {
					_ = dynamicClient.Resource(gvr).Namespace(u.GetNamespace()).Delete(ctx, u.GetName(), metav1.DeleteOptions{})
				}()

				// load expected config
				expectedRegArgs, err := loadBootstrapConfig(expect, true)
				Expect(err).ToNot(HaveOccurred())

				// wait for dynamic config to be applied and compare
				Eventually(func(g Gomega) {
					var got meshregbootstrap.RegistryArgs
					g.Expect(gotCh).Should(Receive(&got))
					Expect(got).Should(Equal(expectedRegArgs))
				})
			},
			Entry("legacy gateway mode full configurator",
				"testdata/legacy-gw-mode.static.yaml", "testdata/legacy-gw-mode.dynamic.cm.yaml", "testdata/legacy-gw-mode.expect.cm.yaml", //nolint: lll
				cmGvr, setFullConfigurator),
			Entry("legacy gateway mode patch configurator",
				"testdata/legacy-gw-mode.static.yaml", "testdata/legacy-gw-mode.dynamic.cr.yaml", "testdata/legacy-gw-mode.expect.cr.yaml", //nolint: lll
				crGvr, setPatchConfigurator),
		)
	})
})

func loadBootstrapConfig(path string, directly bool) (*meshregbootstrap.RegistryArgs, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if directly {
		var cfg meshregbootstrap.RegistryArgs
		err = yaml.Unmarshal(data, &cfg)
		if err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	cfgWrapper, err := parseModuleConfig(data)
	if err != nil {
		return nil, err
	}
	return ParseArgsFromModuleConfig(cfgWrapper)
}
