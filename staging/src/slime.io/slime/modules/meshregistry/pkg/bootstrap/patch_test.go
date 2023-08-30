package bootstrap

import (
	"encoding/json"
	"testing"
)

func TestPatch(t *testing.T) {
	equal := func(a, b *RegistryArgs) (string, string, bool) {
		gota, _ := json.Marshal(a)
		gotb, _ := json.Marshal(b)
		return string(gota), string(gotb), string(gota) == string(gotb)
	}
	tests := []struct {
		name   string
		target *RegistryArgs
		patch  *RegistryArgs
		want   *RegistryArgs
	}{
		{
			name: "patch nil zookeeper",
			target: &RegistryArgs{
				ZookeeperSource: &ZookeeperSourceArgs{
					SourceArgs: SourceArgs{
						Enabled: true,
					},
				},
			},
			patch: &RegistryArgs{},
			want: &RegistryArgs{
				ZookeeperSource: &ZookeeperSourceArgs{
					SourceArgs: SourceArgs{
						Enabled: true,
					},
				},
			},
		},
		{
			name: "patch zookeeper",
			target: &RegistryArgs{
				ZookeeperSource: &ZookeeperSourceArgs{
					SourceArgs: SourceArgs{
						Enabled: true,
						EndpointSelectors: []*EndpointSelector{
							{
								ExcludeIPRanges: &IPRanges{
									IPs: []string{"1.1.1.1"},
								},
							},
						},
						ServicedEndpointSelectors: map[string][]*EndpointSelector{
							"i:g:v": {
								{
									ExcludeIPRanges: &IPRanges{
										IPs: []string{"2.2.2.2"},
									},
								},
							},
						},
					},
				},
			},
			patch: &RegistryArgs{
				ZookeeperSource: &ZookeeperSourceArgs{
					SourceArgs: SourceArgs{
						EndpointSelectors: []*EndpointSelector{
							{
								ExcludeIPRanges: &IPRanges{
									IPs: []string{"1.1.1.2"},
								},
							},
						},
						ServicedEndpointSelectors: map[string][]*EndpointSelector{
							"i:g:v": {
								{
									ExcludeIPRanges: &IPRanges{
										IPs: []string{"2.2.2.3"},
									},
								},
							},
							"i:g:v2": {
								{
									ExcludeIPRanges: &IPRanges{
										IPs: []string{"3.3.3.3"},
									},
								},
							},
						},
					},
				},
			},
			want: &RegistryArgs{
				ZookeeperSource: &ZookeeperSourceArgs{
					SourceArgs: SourceArgs{
						Enabled: true,
						EndpointSelectors: []*EndpointSelector{
							{
								ExcludeIPRanges: &IPRanges{
									IPs: []string{"1.1.1.1"},
								},
							},
							{
								ExcludeIPRanges: &IPRanges{
									IPs: []string{"1.1.1.2"},
								},
							},
						},
						ServicedEndpointSelectors: map[string][]*EndpointSelector{
							"i:g:v": {
								{
									ExcludeIPRanges: &IPRanges{
										IPs: []string{"2.2.2.3"},
									},
								},
							},
							"i:g:v2": {
								{
									ExcludeIPRanges: &IPRanges{
										IPs: []string{"3.3.3.3"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Patch(tt.target, tt.patch)
			if got, want, ok := equal(tt.target, tt.want); !ok {
				t.Errorf("Patch() = %v, want %v", got, want)
			}
		})
	}
}
