/*
* @Author: yangdihang
* @Date: 2020/11/4
 */

package pluginmanager

import (
	"context"
	"io/ioutil"
	"testing"

	"gopkg.in/yaml.v2"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"slime.io/slime/pkg/apis/microservice/v1alpha1/wrapper"
	"slime.io/slime/pkg/apis/networking/v1alpha3"
	"slime.io/slime/pkg/controller"
	"slime.io/slime/test"
)

func readPluginManager(file string) *wrapper.PluginManager {
	t := &wrapper.PluginManager{}
	if d, err := ioutil.ReadFile(file); err == nil {
		yaml.Unmarshal(d, t)
	}
	return t
}

func readEnvoyFilter(file string) *v1alpha3.EnvoyFilter {
	t := &v1alpha3.EnvoyFilter{}
	if d, err := ioutil.ReadFile(file); err == nil {
		yaml.Unmarshal(d, t)
	}
	return t
}

func initEnv(file string, name string, namespace string) *ReconcilePluginManager {
	controller.UpdateHook[controller.PluginManager] = []func(object meta_v1.Object, args ...interface{}) error{DoUpdate}
	m := test.MockClient{}
	ep1 := readPluginManager(file)
	ep1.Name = name
	ep1.Namespace = namespace
	m.Object = map[string]map[client.ObjectKey]runtime.Object{
		"*v1alpha1.PluginManager": {
			client.ObjectKey{
				Namespace: namespace,
				Name:      name,
			}: ep1,
		},
	}
	return &ReconcilePluginManager{client: &m}
}

func expect(file string, name string, namespace string) *v1alpha3.EnvoyFilter {
	e := readEnvoyFilter(file)
	e.Name = name
	e.Namespace = namespace
	return e
}

func testcase(pluginmanager, envoyfilter, name, namespace string, t *testing.T) {
	r := initEnv(pluginmanager, name, namespace)
	l := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	_, err := r.Reconcile(reconcile.Request{NamespacedName: l})
	if err != nil {
		t.Fatalf("Reconcile err :%v", err)
	}
	actual := &v1alpha3.EnvoyFilter{}
	err = r.client.Get(context.TODO(), l, actual)
	if err != nil {
		t.Fatalf("Reconcile err :%v", err)
	}
	expected := expect(envoyfilter, name, namespace)

	if !test.IsEqual(expected, actual) {
		t.Fatalf("expected is %v, but got %v", expected, actual)
	}
}

func TestReconcile(t *testing.T) {
	testcase("test/p1.yaml", "test/f1.yaml", "p1", "test", t)
}
