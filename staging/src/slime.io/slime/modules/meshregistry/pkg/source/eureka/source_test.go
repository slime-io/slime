package eureka

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source/sourcetest"
)

func TestUpdateServiceInfo(t *testing.T) {
	type testData struct {
		// clientErr is the error returned by the mock client
		clientErr error
		// in is the json format input file path
		in string
		// expect is the yaml format expected file path
		expect string
	}
	assertHandler := sourcetest.NewAssertEventHandler()
	mockClient := &MockClient{}
	args := []struct {
		name           string
		data           []testData
		s              *Source
		enableFeatures func()
	}{
		{
			name: "simple",
			data: []testData{
				{
					in:     "./testdata/simple.json",
					expect: "./testdata/simple.expected.yaml",
				},
			},
			s: &Source{
				args: &bootstrap.EurekaSourceArgs{
					SourceArgs: bootstrap.SourceArgs{
						SvcProtocol:           "http",
						InstancePortAsSvcPort: true,
						ResourceNs:            "eureka",
						DefaultServiceNs:      "eureka",
					},
				},
			},
		},
		{
			name: "simple-scale-down-instance",
			data: []testData{
				{
					in:     "./testdata/simple.json",
					expect: "./testdata/simple.expected.yaml",
				},
				{
					in:     "./testdata/simple_scale_down_instance.json",
					expect: "./testdata/simple_scale_down_instance.expected.yaml",
				},
			},
			s: &Source{
				args: &bootstrap.EurekaSourceArgs{
					SourceArgs: bootstrap.SourceArgs{
						SvcProtocol:           "http",
						InstancePortAsSvcPort: true,
						ResourceNs:            "eureka",
						DefaultServiceNs:      "eureka",
					},
				},
			},
		},
		{
			name: "legacy_gateway_mode",
			data: []testData{
				{
					in:     "./testdata/legacy_gateway_mode.json",
					expect: "./testdata/legacy_gateway_mode.expected.yaml",
				},
			},
			s: &Source{
				args: &bootstrap.EurekaSourceArgs{
					SourceArgs: bootstrap.SourceArgs{
						SvcProtocol:           "http",
						SvcPort:               80,
						InstancePortAsSvcPort: false,
						ResourceNs:            "eureka",
						DefaultServiceNs:      "eureka",
					},
					NsHost: true,
				},
			},
		},
		{
			name: "enable-project-code",
			data: []testData{
				{
					in:     "./testdata/project_code.json",
					expect: "./testdata/project_code.expected.yaml",
				},
			},
			s: &Source{
				args: &bootstrap.EurekaSourceArgs{
					SourceArgs: bootstrap.SourceArgs{
						SvcProtocol:           "http",
						SvcPort:               80,
						InstancePortAsSvcPort: false,
						ResourceNs:            "eureka",
						DefaultServiceNs:      "eureka",
					},
					NsHost:            true,
					EnableProjectCode: true,
					AppSuffix:         "nsf",
				},
			},
		},
	}

	initTest := func(s *Source) {
		mockClient.Reset()
		s.client = mockClient
		assertHandler.Reset()
		s.Dispatch(assertHandler)
	}

	for _, tt := range args {
		initTest(tt.s)
		t.Run(tt.name, func(t *testing.T) {
			for _, d := range tt.data {
				if d.clientErr != nil {
					mockClient.SetError(d.clientErr)
					assert.Error(t, tt.s.updateServiceInfo())
					continue
				}
				if d.in != "" {
					assert.NoError(t, mockClient.Load(d.in))
				}
				if d.expect != "" {
					assertHandler.Reset()
					assert.NoError(t, assertHandler.LoadExpected(d.expect))
				}
				assert.NoError(t, tt.s.updateServiceInfo())
				assertHandler.Assert(t)
			}
		})
	}
}
