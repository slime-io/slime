package source

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

func TestEndpointSelector(t *testing.T) {
	getPointerOfStr := func(s string) *string {
		return &s
	}

	var args = []struct {
		name                 string
		endpointSelectors    map[string][]*bootstrap.EndpointSelector
		emptySelectorsReturn bool
		cases                []struct {
			name      string
			svc       string
			hookParam HookParam
			expected  bool
		}
	}{
		{
			name: "test label selector",
			endpointSelectors: map[string][]*bootstrap.EndpointSelector{
				"foo": {
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "foo",
							},
						},
						ExcludeIPRanges: &bootstrap.IPRanges{
							IPs: []string{
								"1.1.1.1",
							},
						},
					},
				},
				"bar": {
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "bar",
							},
						},
						ExcludeIPRanges: &bootstrap.IPRanges{
							IPs: []string{
								"2.2.2.2",
							},
						},
					},
				},
			},
			cases: []struct {
				name      string
				svc       string
				hookParam HookParam
				expected  bool
			}{
				{
					name: "label match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "foo",
						},
					},
					expected: true,
				},
				{
					name: "label not match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "test",
						},
					},
					expected: false,
				},
				{
					name: "ip match",
					svc:  "foo",
					hookParam: HookParam{
						IP: getPointerOfStr("2.2.2.2"),
					},
					expected: true,
				},
				{
					name: "ip not match",
					svc:  "foo",
					hookParam: HookParam{
						IP: getPointerOfStr("1.1.1.1"),
					},
					expected: false,
				},
				{
					name: "label match and ip match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "foo",
						},
						IP: getPointerOfStr("2.2.2.2"),
					},
					expected: true,
				},
				{
					name: "label match and ip not match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "foo",
						},
						IP: getPointerOfStr("1.1.1.1"),
					},
					expected: false,
				},
			},
		},
	}
	for _, arg := range args {
		t.Run(arg.name, func(t *testing.T) {
			var store HookStore
			cfgs := make(map[string]HookConfig, len(arg.endpointSelectors))
			for k, v := range arg.endpointSelectors {
				cfgs[k] = ConvertEndpointSelectorToHookConfig(v, HookConfigWithEmptySelectorsReturn(arg.emptySelectorsReturn))
			}
			for _, c := range arg.cases {
				t.Run(c.name, func(t *testing.T) {
					if h, ok := store[c.svc]; ok {
						if h(c.hookParam) != c.expected {
							t.Errorf("expected %v, got %v", c.expected, !c.expected)
						}
					}
				})
			}
		})
	}

}
