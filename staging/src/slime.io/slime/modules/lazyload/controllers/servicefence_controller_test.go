package controllers

import (
	"context"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/testing/protocmp"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap"
	basecontroller "slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/modules/lazyload/api/config"
)

var _ = Describe("ServicefenceReconciler", func() {
	var ctx context.Context
	var cancel context.CancelFunc
	var mgr manager.Manager
	var mgrClient client.Client

	BeforeEach(func() {
		var err error
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme,
		})
		Expect(err).NotTo(HaveOccurred())
		mgrClient = mgr.GetClient()
	})

	Describe("metric accesslog with enable autofence", func() {
		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			opts := module.ModuleOptions{
				Env: bootstrap.Environment{
					K8SClient:     k8sClient,
					DynamicClient: dynamicClient,
					Config: &bootconfig.Config{
						Global: &bootconfig.Global{
							IstioNamespace: "istio-system",
							SlimeNamespace: "mesh-operator",
							Misc: map[string]string{
								"logSourcePort": "20000",
							},
						},
					},
					Stop: ctx.Done(),
				},
				Manager: mgr,
			}
			moduleCfg := config.Fence{
				MetricSourceType:   MetricSourceTypeAccesslog,
				ClusterGsNamespace: "mesh-operator",
				AutoFence:          true,
				DefaultFence:       true,
			}

			sfr, baseBuilder, err := newTestReconciler(ctx, opts, moduleCfg)
			Expect(err).NotTo(HaveOccurred())

			// enable autofence
			builder := baseBuilder.Add(basecontroller.ObjectReconcileItem{
				Name:    "Namespace",
				ApiType: &corev1.Namespace{},
				R:       reconcile.Func(sfr.ReconcileNamespace),
			}).Add(basecontroller.ObjectReconcileItem{
				Name:    "Service",
				ApiType: &corev1.Service{},
				R:       reconcile.Func(sfr.ReconcileService),
			})

			err = builder.Build(mgr)
			Expect(err).NotTo(HaveOccurred())

			go func() {
				sfr.WatchMetric(ctx)
			}()

			go func() {
				defer func() {
					GinkgoRecover()
				}()
				err := mgr.Start(ctx)
				Expect(err).NotTo(HaveOccurred())
			}()
		})

		AfterEach(func() {
			cancel()
			ctx, cancel, mgr, mgrClient = nil, nil, nil, nil
		})

		Describe("test autofence", Ordered, func() {
			It("test sidecar generate", func() {
				input := "./testdata/autofence.base.input.yaml"
				expect := "./testdata/autofence.base.expect.sidecar.yaml"
				objs, err := loadYamlObjects(input)
				Expect(err).NotTo(HaveOccurred())
				for _, obj := range objs {
					err := mgrClient.Create(ctx, obj)
					Expect(err).NotTo(HaveOccurred())
				}

				var want networkingv1alpha3.Sidecar
				err = loadYamlTestData(&want, expect)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					got := &networkingv1alpha3.Sidecar{}
					err := mgrClient.Get(ctx, types.NamespacedName{
						Name:      want.ObjectMeta.Name,
						Namespace: want.ObjectMeta.Namespace,
					}, got)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(got.ObjectMeta.Labels).To(Equal(want.ObjectMeta.Labels))
					g.Expect(got.ObjectMeta.Annotations).To(Equal(want.ObjectMeta.Annotations))
					g.Expect(cmp.Diff(&got.Spec, &want.Spec, protocmp.Transform())).To(BeEmpty())
				}).WithTimeout(5 * time.Second).Should(Succeed())
			})
			// TODO: add test for handle accesslog and check the sidecar update
		})
	})
})

func newTestReconciler(ctx context.Context, opts module.ModuleOptions, moduleCfg config.Fence,
) (*ServicefenceReconciler, *basecontroller.ObjectReconcilerBuilder, error) {
	env, mgr := opts.Env, opts.Manager
	pc, err := NewProducerConfig(env, moduleCfg)
	if err != nil {
		return nil, nil, err
	}

	sfr := NewReconciler(
		ReconcilerWithCfg(&moduleCfg),
		ReconcilerWithEnv(env),
		ReconcilerWithProducerConfig(pc),
	)
	sfr.Client = mgr.GetClient()
	sfr.Scheme = mgr.GetScheme()

	sfr.StartCache(ctx)
	cache, err := NewCache(env)
	if err != nil {
		return nil, nil, err
	}
	source := metric.NewSource(pc)
	err = source.Fullfill(cache)
	if err != nil {
		return nil, nil, err
	}
	var builder basecontroller.ObjectReconcilerBuilder
	builder = builder.Add(basecontroller.ObjectReconcileItem{
		Name: "ServiceFence",
		R:    sfr,
	}).Add(basecontroller.ObjectReconcileItem{
		Name: "VirtualService",
		R: &basecontroller.VirtualServiceReconciler{
			Env:    &env,
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
	})
	metric.NewProducer(pc, source)
	return sfr, &builder, nil
}
