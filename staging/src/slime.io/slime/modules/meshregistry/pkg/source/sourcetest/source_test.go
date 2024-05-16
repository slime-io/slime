package sourcetest

import (
	"reflect"
	"sort"
	"testing"
)

func TestZkNode_GetChildren(t *testing.T) {
	root := &ZkNode{
		FullPath: "/",
		Children: map[string]*ZkNode{
			"a": {
				FullPath: "/a",
			},
			"b": {
				FullPath: "/b",
				Children: map[string]*ZkNode{
					"c": {
						FullPath: "/b/c",
					},
				},
			},
			"bb": {
				FullPath: "/bb",
				Children: map[string]*ZkNode{
					"cc": {
						FullPath: "/bb/cc",
					},
				},
			},
			"d": {
				FullPath: "/d",
				Children: map[string]*ZkNode{
					"dd": {
						FullPath: "/d/dd",
						Children: map[string]*ZkNode{
							"ddd": {
								FullPath: "/d/dd/ddd",
							},
						},
					},
				},
			},
		},
	}
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "root",
			args: args{
				path: "/",
			},
			want: []string{
				"a",
				"b",
				"bb",
				"d",
			},
		},
		{
			name: "b",
			args: args{
				path: "/b",
			},
			want: []string{
				"c",
			},
		},
		{
			name: "parent is exist, but has no children",
			args: args{
				path: "/a",
			},
			want: nil,
		},
		{
			name: "parent is not exist",
			args: args{
				path: "/not_exist",
			},
			want: nil,
		},
		{
			name: "deep path",
			args: args{
				path: "/d/dd",
			},
			want: []string{
				"ddd",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := root.GetChildren(tt.args.path)
			sort.Strings(got)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ZkNode.GetChildren() = %v, want %v", got, tt.want)
			}
		})
	}
}
