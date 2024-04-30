package source

import (
	"reflect"
	"testing"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

func TestBuildInstanceMetaModifier(t *testing.T) {
	type args struct {
		rl *bootstrap.InstanceMetaRelabel
	}
	tests := []struct {
		name     string
		args     args
		meta     map[string]string
		wantMeta map[string]string
	}{
		{
			name: "relabel with mapping value",
			args: args{
				rl: &bootstrap.InstanceMetaRelabel{
					Items: []*bootstrap.InstanceMetaRelabelItem{
						{
							Key:       "foo",
							TargetKey: "foo1",
							ValuesMapping: map[string]string{
								"bar": "baz",
							},
						},
					},
				},
			},
			meta: map[string]string{
				"foo": "bar",
			},
			wantMeta: map[string]string{
				"foo":  "bar",
				"foo1": "baz",
			},
		},
		{
			name: "relabel without overwrite",
			args: args{
				rl: &bootstrap.InstanceMetaRelabel{
					Items: []*bootstrap.InstanceMetaRelabelItem{
						{
							Key:       "foo",
							TargetKey: "foo1",
						},
					},
				},
			},
			meta: map[string]string{
				"foo":  "bar",
				"foo1": "bar1",
			},
			wantMeta: map[string]string{
				"foo":  "bar",
				"foo1": "bar1",
			},
		},
		{
			name: "relabel with overwrite",
			args: args{
				rl: &bootstrap.InstanceMetaRelabel{
					Items: []*bootstrap.InstanceMetaRelabelItem{
						{
							Key:       "foo",
							TargetKey: "foo1",
							Overwrite: true,
						},
					},
				},
			},
			meta: map[string]string{
				"foo":  "bar",
				"foo1": "bar1",
			},
			wantMeta: map[string]string{
				"foo":  "bar",
				"foo1": "bar",
			},
		},
		{
			name: "create new label with nil map",
			args: args{
				rl: &bootstrap.InstanceMetaRelabel{
					Items: []*bootstrap.InstanceMetaRelabelItem{
						{
							Key:              "foo",
							TargetKey:        "foo",
							CreatedWithValue: "bar",
						},
					},
				},
			},
			wantMeta: map[string]string{
				"foo": "bar",
			},
		},
		{
			name: "create new label with empty map",
			args: args{
				rl: &bootstrap.InstanceMetaRelabel{
					Items: []*bootstrap.InstanceMetaRelabelItem{
						{
							Key:              "foo",
							TargetKey:        "foo",
							CreatedWithValue: "bar",
						},
					},
				},
			},
			meta: map[string]string{},
			wantMeta: map[string]string{
				"foo": "bar",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildInstanceMetaModifier(tt.args.rl)
			if got(&tt.meta); !reflect.DeepEqual(tt.meta, tt.wantMeta) {
				t.Errorf("after do modifiy = %v, want %v", tt.meta, tt.wantMeta)
			}
		})
	}
}
