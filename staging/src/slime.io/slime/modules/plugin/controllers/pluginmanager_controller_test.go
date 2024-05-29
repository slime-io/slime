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
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/testing/protocmp"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/types"

	pluginv1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1"
)

var _ = Describe("PluginManagerReconciler", func() {
	DescribeTable("conversion",
		func(input, expect string) {
			pluginManager := &pluginv1alpha1.PluginManager{}
			// load yaml test pluginmanager
			Expect(loadYamlTestData(pluginManager, input)).Should(Succeed())
			// apply pluginmanager to k8s
			Expect(k8sClient.Create(ctx, pluginManager)).Should(Succeed())
			// load expect envoyfilter
			want := &networkingv1alpha3.EnvoyFilter{}
			Expect(loadYamlTestData(want, expect)).Should(Succeed())

			// get envoyfilter from k8s
			key := types.NamespacedName{
				Name:      pluginManager.Name,
				Namespace: pluginManager.Namespace,
			}

			Eventually(func(g Gomega) {
				got := &networkingv1alpha3.EnvoyFilter{}
				err := k8sClient.Get(ctx, key, got)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(got.ObjectMeta.Labels).To(Equal(want.ObjectMeta.Labels))
				g.Expect(got.ObjectMeta.Annotations).To(Equal(want.ObjectMeta.Annotations))
				g.Expect(cmp.Diff(&got.Spec, &want.Spec, protocmp.Transform())).To(BeEmpty())
			}).Should(Succeed())
		},
		Entry("gateway_sample", "./testdata/gateway_sample.plm.yaml", "./testdata/gateway_sample.plm.expect.yaml"),
	)
})
