/*
* @Author: yangdihang
* @Date: 2020/11/4
 */

package test

import (
	"context"
	"fmt"
	"reflect"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"slime.io/slime/pkg/apis/microservice/v1alpha1/wrapper"
)

type MockClient struct {
	Object map[string]map[client.ObjectKey]runtime.Object
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	typ := reflect.TypeOf(obj).String()
	if m.Object != nil {
		if kindmap, ok := m.Object[typ]; ok {
			if o, ok := kindmap[key]; ok {
				r := convertToNeteaseType(obj)
				t := convertToNeteaseType(o)
				r.SetSpec(t.GetSpec())
				r.SetObjectMeta(t.GetObjectMeta())
			}
		}
	}
	return nil
}

func convertToNeteaseType(obj runtime.Object) wrapper.NeteaseType {
	var i interface{}
	i = obj
	return i.(wrapper.NeteaseType)
}

func (m *MockClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	panic("implement me")
}

func (m *MockClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	panic("implement me")
}

func (m *MockClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	panic("implement me")
}

func (m *MockClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	typ := reflect.TypeOf(obj).String()
	if m.Object != nil {
		if m.Object[typ] == nil {
			m.Object[typ] = make(map[client.ObjectKey]runtime.Object)
		}
		var i interface{}
		i = obj
		meta := i.(meta_v1.Object)
		key := client.ObjectKey{
			Name:      meta.GetName(),
			Namespace: meta.GetNamespace(),
		}
		m.Object[typ][key] = obj
	}
	return nil
}

func (m *MockClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	panic("implement me")
}

func (m *MockClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	panic("implement me")
}

func (m *MockClient) Status() client.StatusWriter {
	panic("implement me")
}

func IsEqual(a, b interface{}) bool {
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return as == bs
}
