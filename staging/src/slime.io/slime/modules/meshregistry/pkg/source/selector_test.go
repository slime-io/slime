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
			name: "test empty selector with emptySelectorsReturn=false",
			endpointSelectors: map[string][]*bootstrap.EndpointSelector{
				"foo": nil,
			},
			emptySelectorsReturn: false,
			cases: []struct {
				name      string
				svc       string
				hookParam HookParam
				expected  bool
			}{
				{
					name: "default empty selector return true",
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
		{
			name: "test empty selector with emptySelectorsReturn=true",
			endpointSelectors: map[string][]*bootstrap.EndpointSelector{
				"foo": nil,
			},
			emptySelectorsReturn: true,
			cases: []struct {
				name      string
				svc       string
				hookParam HookParam
				expected  bool
			}{
				{
					name: "default empty selector return true",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "foo",
						},
						IP: getPointerOfStr("1.1.1.1"),
					},
					expected: true,
				},
			},
		},
		{
			name:                 "test normal selector with emptySelectorsReturn=true",
			emptySelectorsReturn: true,
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
					},
				},
				"baz": {
					{
						ExcludeIPRanges: &bootstrap.IPRanges{
							IPs: []string{
								"1.1.1.1",
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
					name: "label match, ip match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "foo",
						},
						// empty ip never can be excluded
					},
					expected: true,
				},
				{
					name: "label not match, ip match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "test",
						},
						// empty ip never can be excluded
					},
					expected: false,
				},
				{
					name: "label match, ip match 2",
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
					name: "label match, ip not match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "foo",
						},
						IP: getPointerOfStr("1.1.1.1"),
					},
					expected: false,
				},
				{
					name: "label not match, ip not match",
					svc:  "foo",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "test",
						},
						IP: getPointerOfStr("1.1.1.1"),
					},
					expected: false,
				},
				{
					name: "label match, wihout ip selector",
					svc:  "bar",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "bar",
						},
						IP: getPointerOfStr("2.2.2.2"),
					},
					expected: true,
				},
				{
					name: "label match, wihout ip selector 2",
					svc:  "bar",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "bar",
						},
					},
					expected: true,
				},
				{
					name: "label not match, wihout ip selector",
					svc:  "bar",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "foo",
						},
						IP: getPointerOfStr("2.2.2.2"),
					},
					expected: false,
				},
				{
					name: "ip match, wihout label selector",
					svc:  "baz",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "baz",
						},
						IP: getPointerOfStr("3.3.3.3"),
					},
					expected: true,
				},
				{
					name: "ip not match, wihout label selector",
					svc:  "baz",
					hookParam: HookParam{
						Label: map[string]string{
							"app": "baz",
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
			store = NewHookStore(cfgs)
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
