package source

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	networking "istio.io/api/networking/v1alpha3"
)

func TestRectifyServiceEntry(t *testing.T) {
	type args struct {
		se          *networking.ServiceEntry
		rectifiedSe *networking.ServiceEntry
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "string slice",
			args: args{
				se: &networking.ServiceEntry{
					Hosts:    []string{"foo", "bar"},
					ExportTo: []string{"ns2", "ns1"},
				},
				rectifiedSe: &networking.ServiceEntry{
					Hosts:    []string{"bar", "foo"},
					ExportTo: []string{"ns1", "ns2"},
				},
			},
		},
		{
			name: "ports",
			args: args{
				se: &networking.ServiceEntry{
					Ports: []*networking.ServicePort{
						{Number: 81},
						{Number: 80},
					},
				},
				rectifiedSe: &networking.ServiceEntry{
					Ports: []*networking.ServicePort{
						{Number: 80},
						{Number: 81},
					},
				},
			},
		},
		{
			name: "endpoints",
			args: args{
				se: &networking.ServiceEntry{
					Endpoints: []*networking.WorkloadEntry{
						{Address: "2.2.2.2"},
						{Address: "1.1.1.1"},
					},
				},
				rectifiedSe: &networking.ServiceEntry{
					Endpoints: []*networking.WorkloadEntry{
						{Address: "1.1.1.1"},
						{Address: "2.2.2.2"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RectifyServiceEntry(tt.args.se)
			if !proto.Equal(tt.args.se, tt.args.rectifiedSe) {
				t.Errorf("%s: proto not equal after rectify", tt.name)
			}
		})
	}
}
