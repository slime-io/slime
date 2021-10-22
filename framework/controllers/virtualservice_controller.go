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

	log "github.com/sirupsen/logrus"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	networkingistioiov1alpha3 "slime.io/slime/framework/apis/networking/v1alpha3"
)

const (
	vsHosts       = "hosts"
	vsHost        = "host"
	vsDestination = "destination"
	vsHttp        = "http"
	vsRoute       = "route"
)

// VirtualServiceReconciler reconciles a VirtualService object
type VirtualServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Env    *bootstrap.Environment
}

// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch

func (r *VirtualServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	log := log.WithField("virtualService", req.NamespacedName)
	// Fetch the VirtualService instance
	instance := &networkingistioiov1alpha3.VirtualService{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO del event not handled. should re-calc the data accordingly
			log.Infof("virtualService is deleted")
			return reconcile.Result{}, nil
		} else {
			log.Errorf("get virtualService error, %+v", err)
			return reconcile.Result{}, err
		}
	}
	if !model.LabelMatchIstioRev(instance.Labels, r.Env.IstioRev()) {
		return ctrl.Result{}, nil
	}

	// 资源更新
	m := parseDestination(instance)
	log.Infof("get destination after parse, %+v", m)
	for k, v := range m {
		HostDestinationMapping.Set(k, v)
	}

	return ctrl.Result{}, nil
}

func parseDestination(instance *networkingistioiov1alpha3.VirtualService) map[string][]string {
	ret := make(map[string][]string)

	hosts := make([]string, 0)
	i, ok := instance.Spec[vsHosts].([]interface{})
	if !ok {
		return nil
	}
	for _, iv := range i {
		hosts = append(hosts, iv.(string))
	}

	dhs := make(map[string]struct{}, 0)

	if httpRoutes, ok := instance.Spec[vsHttp].([]interface{}); ok {
		for _, httpRoute := range httpRoutes {
			if hr, ok := httpRoute.(map[string]interface{}); ok {
				if ds, ok := hr[vsRoute].([]interface{}); ok {
					for _, d := range ds {
						if route, ok := d.(map[string]interface{}); ok {
							destinationHost := route[vsDestination].(map[string]interface{})[vsHost].(string)
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
