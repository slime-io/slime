package controllers

import (
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/testing/protocmp"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"slime.io/slime/framework/bootstrap/resource"
	limiterv1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
)

var _ = Describe("SmartlimiterReconciler", func() {
	DescribeTable("local_ratelimit_conversiion",
		func(base, input, expect string) {
			if base != "" {
				// load base input
				objs, err := loadYamlObjects(base)
				Expect(err).ToNot(HaveOccurred())
				for _, obj := range objs {
					err = k8sClient.Create(ctx, obj)
					Expect(err).ToNot(HaveOccurred())
					if pod, ok := obj.(*corev1.Pod); ok {
						pod.Status.Phase = corev1.PodRunning
						err = k8sClient.Status().Update(ctx, pod)
					}
				}

				// check the config is stored in k8s and synced to slime env
				Eventually(func(g Gomega) {
					for _, obj := range objs {
						o := obj.DeepCopyObject().(client.Object)
						err = k8sClient.Get(ctx, client.ObjectKey{Name: o.GetName(), Namespace: o.GetNamespace()}, o)
						g.Expect(err).ToNot(HaveOccurred())
						gvks, _, err := scheme.ObjectKinds(obj)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(len(gvks)).ShouldNot(BeZero())
						gvk := resource.GroupVersionKind{
							Group:   gvks[0].Group,
							Version: gvks[0].Version,
							Kind:    gvks[0].Kind,
						}
						res := slimeEnv.ConfigController.Get(gvk, o.GetName(), o.GetNamespace())
						g.Expect(res).ShouldNot(BeNil())
					}
				}).Should(Succeed())
				defer func() {
					for _, obj := range objs {
						_ = k8sClient.Delete(ctx, obj)
					}
				}()
			}

			limiter := &limiterv1alpha2.SmartLimiter{}
			// load yaml test smartlimiter
			Expect(loadYamlTestData(limiter, input)).Should(Succeed())
			// apply smartlimiter to k8s
			Expect(k8sClient.Create(ctx, limiter)).Should(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, limiter)
			}()
			// load expect envoyfilter
			want := &networkingv1alpha3.EnvoyFilter{}
			Expect(loadYamlTestData(want, expect)).Should(Succeed())

			key := types.NamespacedName{
				Name:      limiter.Name + "." + limiter.Namespace + ".ratelimit",
				Namespace: limiter.Namespace,
			}
			Eventually(func(g Gomega) {
				got := &networkingv1alpha3.EnvoyFilter{}
				err := k8sClient.Get(ctx, key, got)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(got.ObjectMeta.Labels).To(Equal(want.ObjectMeta.Labels))
				g.Expect(got.ObjectMeta.Annotations).To(Equal(want.ObjectMeta.Annotations))
				g.Expect(cmp.Diff(&got.Spec, &want.Spec, protocmp.Transform())).To(BeEmpty())
			}).Should(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, want)
			}()
		},
		Entry("method",
			"./testdata/base_objs.local.yaml",
			"./testdata/limit_by_method.local.yaml",
			"./testdata/limit_by_method.local.ef.expect.yaml"),
		Entry("query",
			"./testdata/base_objs.local.yaml",
			"./testdata/limit_by_query.local.yaml",
			"./testdata/limit_by_query.local.ef.expect.yaml"),
		Entry("sourceIP",
			"./testdata/base_objs.local.yaml",
			"./testdata/limit_by_sourceIP.local.yaml",
			"./testdata/limit_by_sourceIP.local.ef.expect.yaml"),
		Entry("gateway-with-selector-but-mismatch",
			"./testdata/base_objs.local.yaml",
			"./testdata/limit_outbound_with_mismatch_selector.local.yaml",
			"./testdata/limit_outbound_with_mismatch_selector.local.ef.expect.yaml"),
	)
})
