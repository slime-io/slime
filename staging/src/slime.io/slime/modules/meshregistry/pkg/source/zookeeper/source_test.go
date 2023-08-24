/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package zookeeper

import (
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

func Test_generateInstanceFilter(t *testing.T) {
	type args struct {
		svcSel                           map[string][]*bootstrap.EndpointSelector
		epSel                            []*bootstrap.EndpointSelector
		emptySelectorsReturn             bool
		alwaysUseSourceScopedEpSelectors bool
	}
	tests := []struct {
		name string
		args args
		inst *dubboInstance
		want bool
	}{
		{
			name: "empty selectors return false",
			args: args{
				emptySelectorsReturn:             false,
				alwaysUseSourceScopedEpSelectors: false,
			},
			inst: &dubboInstance{
				metadata: map[string]string{
					"interface": "interface1",
				},
			},
			want: false,
		},
		{
			name: "empty selectors return true",
			args: args{
				emptySelectorsReturn:             true,
				alwaysUseSourceScopedEpSelectors: false,
			},
			inst: &dubboInstance{
				metadata: map[string]string{
					"interface": "interface1",
				},
			},
			want: true,
		},
		{
			name: "source scoped ep selectors without ExcludeIPRanges",
			args: args{
				emptySelectorsReturn:             false,
				alwaysUseSourceScopedEpSelectors: false,
				epSel: []*bootstrap.EndpointSelector{
					{
						LabelSelector: &v1.LabelSelector{
							MatchLabels: map[string]string{
								"interface": "interface1",
							},
						},
					},
				},
			},
			inst: &dubboInstance{
				metadata: map[string]string{
					"interface": "interface1",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateInstanceFilter(tt.args.svcSel, tt.args.epSel, tt.args.emptySelectorsReturn, tt.args.alwaysUseSourceScopedEpSelectors)
			if got(tt.inst) != tt.want {
				t.Errorf("generateInstanceFilter() = %v, want %v", got(tt.inst), tt.want)
			}
		})
	}
}
