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
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"sigs.k8s.io/yaml"

	"slime.io/slime/modules/plugin/api/config"
	microserviceslimeiov1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1"
)

func TestEnvoyPluginReconciler_newEnvoyFilterForEnvoyPlugin(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "gateway_rc_patch",
			input: "./testdata/gateway_rc_patch.ep.yaml",
			want:  "./testdata/gateway_rc_patch.ep.expect.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &EnvoyPluginReconciler{
				Cfg: &config.PluginModule{},
			}
			envoyPlugin := &microserviceslimeiov1alpha1.EnvoyPlugin{}
			assert.NoError(t, loadYamlTestData(envoyPlugin, tt.input))

			want := &networkingv1alpha3.EnvoyFilter{}
			assert.NoError(t, loadYamlTestData(want, tt.want))
			got := r.newEnvoyFilterForEnvoyPlugin(envoyPlugin)
			assertEnvoyFilterEqual(t, got, want)
		})
	}
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

func assertEnvoyFilterEqual(t *testing.T, got, want *networkingv1alpha3.EnvoyFilter) {
	assert.Equal(t, got.ObjectMeta.Name, want.ObjectMeta.Name)
	assert.Equal(t, got.ObjectMeta.Namespace, want.ObjectMeta.Namespace)

	assert.Equal(t, got.ObjectMeta.Labels, want.ObjectMeta.Labels)
	assert.Equal(t, got.ObjectMeta.Annotations, want.ObjectMeta.Annotations)

	assert.Equalf(t, true, proto.Equal(&got.Spec, &want.Spec),
		"diff: %s", cmp.Diff(&got.Spec, &want.Spec, protocmp.Transform()))
}
