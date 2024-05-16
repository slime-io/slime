/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package zookeeper

import (
	"testing"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/assert"
	"istio.io/libistio/pkg/config/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/source/sourcetest"
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
				Metadata: map[string]string{
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
				Metadata: map[string]string{
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
				Metadata: map[string]string{
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

func TestUpdateServiceInfo(t *testing.T) {
	type testData struct {
		// clientErr is the error returned by the mock client
		clientErr error
		// in is the yaml format input file path
		in string
		// expect is the yaml format expected file path
		expect string
	}

	newSource := func(args *bootstrap.ZookeeperSourceArgs, opts ...func(*Source)) *Source {
		s := &Source{
			args:                 args,
			serviceMethods:       map[string]string{},
			seDubboCallModels:    map[resource.FullName]map[string]DubboCallModel{},
			appSidecarUpdateTime: map[string]time.Time{},
			registryServiceCache: cmap.New[cmap.ConcurrentMap[string, []dubboInstance]](),
			cache:                cmap.New[cmap.ConcurrentMap[string, *ServiceEntryWithMeta]](),
		}
		if args.MockServiceEntryName != "" && args.MockServiceName != "" {
			s.seMergePortMocker = source.NewServiceEntryMergePortMocker(
				args.MockServiceEntryName, args.ResourceNs, args.MockServiceName,
				args.MockServiceMergeInstancePort, args.MockServiceMergeServicePort,
				map[string]string{
					"path":     args.MockServiceName,
					"registry": SourceName,
				},
			)
			s.seMergePortMocker.SetDispatcher(s.dispatchMergePortsServiceEntry)
			s.handlers = append(s.handlers, s.seMergePortMocker)
		}

		for _, opt := range opts {
			opt(s)
		}
		return s
	}

	assertHandler := sourcetest.NewAssertEventHandler()
	mockClient := &MockZkConn{}
	args := []struct {
		name string
		data []testData
		s    *Source
	}{
		{
			name: "simple",
			data: []testData{
				{
					in:     "./testdata/simple.yaml",
					expect: "./testdata/simple.expected.yaml",
				},
			},
			s: newSource(&bootstrap.ZookeeperSourceArgs{
				SourceArgs: bootstrap.SourceArgs{
					SvcProtocol:           "DUBBO",
					InstancePortAsSvcPort: true,
					ResourceNs:            "dubbo",
					DefaultServiceNs:      "dubbo",
				},
				RegistryRootNode: "/dubbo",
			}),
		},
		{
			name: "simple_scale_down_instance",
			data: []testData{
				{
					in:     "./testdata/simple.yaml",
					expect: "./testdata/simple.expected.yaml",
				},
				{
					in:     "./testdata/simple_scale_down_instance.yaml",
					expect: "./testdata/simple_scale_down_instance.expected.yaml",
				},
			},
			s: newSource(&bootstrap.ZookeeperSourceArgs{
				SourceArgs: bootstrap.SourceArgs{
					SvcProtocol:           "DUBBO",
					InstancePortAsSvcPort: true,
					ResourceNs:            "dubbo",
					DefaultServiceNs:      "dubbo",
				},
				RegistryRootNode: "/dubbo",
			}),
		},
		{
			name: "simple_with_dubbo_call_model",
			data: []testData{
				{
					in:     "./testdata/simple.yaml",
					expect: "./testdata/simple_with_dubbo_call_model.expected.yaml",
				},
			},
			s: newSource(&bootstrap.ZookeeperSourceArgs{
				SourceArgs: bootstrap.SourceArgs{
					SvcProtocol:           "DUBBO",
					InstancePortAsSvcPort: true,
					ResourceNs:            "dubbo",
					DefaultServiceNs:      "dubbo",
				},
				RegistryRootNode:      "/dubbo",
				ConsumerPath:          "/consumers",
				EnableDubboSidecar:    true,
				DubboWorkloadAppLabel: "application",
			}),
		},
		{
			name: "simple_with_dubbo_call_model_and_self_consume",
			data: []testData{
				{
					in:     "./testdata/simple.yaml",
					expect: "./testdata/simple_with_dubbo_call_model_and_self_consume.expected.yaml",
				},
			},
			s: newSource(&bootstrap.ZookeeperSourceArgs{
				SourceArgs: bootstrap.SourceArgs{
					SvcProtocol:           "DUBBO",
					InstancePortAsSvcPort: true,
					ResourceNs:            "dubbo",
					DefaultServiceNs:      "dubbo",
				},
				RegistryRootNode:      "/dubbo",
				ConsumerPath:          "/consumers",
				EnableDubboSidecar:    true,
				SelfConsume:           true,
				DubboWorkloadAppLabel: "application",
			}),
		},
		{
			name: "se_merge_port_mocker",
			data: []testData{
				{
					in:     "./testdata/simple.yaml",
					expect: "./testdata/simple_se_merge_port_mocker.expected.yaml",
				},
			},
			s: newSource(&bootstrap.ZookeeperSourceArgs{
				SourceArgs: bootstrap.SourceArgs{
					SvcProtocol:                 "DUBBO",
					InstancePortAsSvcPort:       true,
					ResourceNs:                  "dubbo",
					DefaultServiceNs:            "dubbo",
					MockServiceName:             "not-really-exist-dubbo.svc",
					MockServiceEntryName:        "not-really-exist-dubbo",
					MockServiceMergeServicePort: true,
				},
				RegistryRootNode: "/dubbo",
			}),
		},
		{
			name: "legacy_gateway_mode",
			data: []testData{
				{
					in:     "./testdata/legacy_gateway_mode.yaml",
					expect: "./testdata/legacy_gateway_mode.expected.yaml",
				},
			},
			s: newSource(&bootstrap.ZookeeperSourceArgs{
				SourceArgs: bootstrap.SourceArgs{
					SvcProtocol:           "http",
					SvcPort:               80,
					InstancePortAsSvcPort: false,
					ResourceNs:            "dubbo",
					DefaultServiceNs:      "dubbo",
				},
				HostSuffix:       ".dubbo",
				RegistryRootNode: "/dubbo",
			}),
		},
		{
			name: "gateway_mode_generic_dubbo",
			data: []testData{
				{
					in:     "./testdata/gateway_mode_generic_dubbo.yaml",
					expect: "./testdata/gateway_mode_generic_dubbo.expected.yaml",
				},
			},
			s: newSource(&bootstrap.ZookeeperSourceArgs{
				SourceArgs: bootstrap.SourceArgs{
					GenericProtocol:       true,
					SvcProtocol:           "DUBBO",
					SvcPort:               80,
					InstancePortAsSvcPort: false,
					ResourceNs:            "dubbo",
					DefaultServiceNs:      "dubbo",
				},
				HostSuffix:       ".dubbo",
				RegistryRootNode: "/dubbo",
			}),
		},
		// TODO: add test cases for:
		// - instance filter
		// - instance meta modifier
		// - method lb checker
		// - ...
	}

	initTest := func(s *Source) {
		mockClient.Reset()
		s.Con = mockClient
		assertHandler.Reset()
		s.Dispatch(assertHandler)
	}

	for _, tt := range args {
		initTest(tt.s)
		t.Run(tt.name, func(t *testing.T) {
			for _, d := range tt.data {
				if d.clientErr != nil {
					assert.Error(t, tt.s.updateServiceInfo())
					continue
				}
				if d.in != "" {
					assert.NoError(t, mockClient.LoadData(d.in))
				}
				if d.expect != "" {
					assertHandler.Reset()
					assert.NoError(t, assertHandler.LoadExpected(d.expect))
				}
				assert.NoError(t, tt.s.updateServiceInfo())
				if tt.s.args.EnableDubboSidecar {
					tt.s.refreshSidecar(false)
				}
				if tt.s.seMergePortMocker != nil {
					tt.s.seMergePortMocker.Refresh()
				}
				assertHandler.DumpGot()
				assertHandler.Assert(t)
			}
		})
	}
}
