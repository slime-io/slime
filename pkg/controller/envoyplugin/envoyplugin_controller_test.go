/*
* @Author: yangdihang
* @Date: 2020/10/23
 */

package envoyplugin

import (
	"context"
	"io/ioutil"
	"testing"

	"slime.io/slime/pkg/apis/microservice/v1alpha1/wrapper"
	"slime.io/slime/pkg/apis/networking/v1alpha3"
	"slime.io/slime/pkg/controller"
	"slime.io/slime/test"

	"gopkg.in/yaml.v2"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func readEnvoyPlugin(file string) *wrapper.EnvoyPlugin {
	t := &wrapper.EnvoyPlugin{}
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

func initEnv(file string, name string, namespace string) *ReconcileEnvoyPlugin {
	controller.UpdateHook[controller.EnvoyPlugin] = []func(object meta_v1.Object, args ...interface{}) error{DoUpdate}
	m := test.MockClient{}
	ep1 := readEnvoyPlugin(file)
	ep1.Name = name
	ep1.Namespace = namespace
	m.Object = map[string]map[client.ObjectKey]runtime.Object{
		"*v1alpha1.EnvoyPlugin": {
			client.ObjectKey{
				Namespace: namespace,
				Name:      name,
			}: ep1,
		},
	}
	return &ReconcileEnvoyPlugin{client: &m}
}

func expect(file string, name string, namespace string) *v1alpha3.EnvoyFilter {
	e := readEnvoyFilter(file)
	e.Name = name
	e.Namespace = namespace
	return e
}

func TestReconcile(t *testing.T) {
	testcase("test/e1.yaml", "test/f1.yaml", "e1", "test", t)
	testcase("test/e2.yaml", "test/f2.yaml", "e2", "test", t)
	testcase("test/e3.yaml", "test/f3.yaml", "e3", "test", t)
}

func testcase(envoyplugin, envoyfilter, name, namespace string, t *testing.T) {
	r := initEnv(envoyplugin, name, namespace)
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
