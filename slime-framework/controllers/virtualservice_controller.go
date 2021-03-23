/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingistioiov1alpha3 "slime.io/slime/slime-framework/apis/networking/v1alpha3"
)

// VirtualServiceReconciler reconciles a VirtualService object
type VirtualServiceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch

func (r *VirtualServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("virtualservice", req.NamespacedName)
	// Fetch the VirtualService instance
	instance := &networkingistioiov1alpha3.VirtualService{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)

	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// 资源删除
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}

	// 资源更新
	m := parseDestination(instance)
	for k, v := range m {
		HostDestinationMapping.Set(k, v)
	}

	return ctrl.Result{}, nil
}

func parseDestination(instance *networkingistioiov1alpha3.VirtualService) map[string][]string {
	ret := make(map[string][]string)

	hosts := make([]string, 0)
	i, ok := instance.Spec["hosts"].([]interface{})
	for _, iv := range i {
		hosts = append(hosts, iv.(string))
	}
	if !ok {
		return nil
	}

	dhs := make(map[string]struct{}, 0)

	if httpRoutes, ok := instance.Spec["http"].([]interface{}); ok {
		for _, httpRoute := range httpRoutes {
			if hr, ok := httpRoute.(map[string]interface{}); ok {
				if ds, ok := hr["route"].([]interface{}); ok {
					for _, d := range ds {
						if route, ok := d.(map[string]interface{}); ok {
							destinationHost := route["destination"].(map[string]interface{})["host"].(string)
							dhs[destinationHost] = struct{}{}
						}
					}
				}
			}
		}
	}

	for _, h := range hosts {
		for dh := range dhs {
			if h != dh {
				if ret[h] == nil {
					ret[h] = []string{dh}
				} else {
					ret[h] = append(ret[h], dh)
				}
			}
		}
	}
	return ret
}

func (r *VirtualServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingistioiov1alpha3.VirtualService{}).
		Complete(r)
}
