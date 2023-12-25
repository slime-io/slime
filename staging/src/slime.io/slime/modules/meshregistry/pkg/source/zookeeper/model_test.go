package zookeeper

import (
	"testing"

	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/resource"
)

func TestServiceEntryWithMeta_Equals(t *testing.T) {
	tests := []struct {
		name string
		sem  ServiceEntryWithMeta
		o    ServiceEntryWithMeta
		want bool
	}{
		{
			name: "different meta version",
			sem: ServiceEntryWithMeta{
				ServiceEntry: &networkingapi.ServiceEntry{
					Hosts: []string{"foo"},
				},
				Meta: resource.Metadata{
					Labels:  resource.StringMap{"foo": "bar"},
					Version: "v1",
				},
			},
			o: ServiceEntryWithMeta{
				ServiceEntry: &networkingapi.ServiceEntry{
					Hosts: []string{"foo"},
				},
				Meta: resource.Metadata{
					Labels:  resource.StringMap{"foo": "bar"},
					Version: "v2",
				},
			},
			want: true,
		},
		{
			name: "different map order",
			sem: ServiceEntryWithMeta{
				ServiceEntry: &networkingapi.ServiceEntry{
					Hosts: []string{"foo"},
				},
				Meta: resource.Metadata{
					Labels:      resource.StringMap{"hello": "world", "foo": "bar"},
					Annotations: resource.StringMap{"hello": "world", "foo": "bar"},
				},
			},
			o: ServiceEntryWithMeta{
				ServiceEntry: &networkingapi.ServiceEntry{
					Hosts: []string{"foo"},
				},
				Meta: resource.Metadata{
					Labels:      resource.StringMap{"foo": "bar", "hello": "world"},
					Annotations: resource.StringMap{"foo": "bar", "hello": "world"},
				},
			},
			want: true,
		},
		{
			name: "different serviceEntry",
			sem: ServiceEntryWithMeta{
				ServiceEntry: &networkingapi.ServiceEntry{
					Hosts: []string{"foo"},
				},
			},
			o: ServiceEntryWithMeta{
				ServiceEntry: &networkingapi.ServiceEntry{
					Hosts: []string{"bar"},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sem.Equals(tt.o); got != tt.want {
				t.Errorf("ServiceEntryWithMeta.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}
