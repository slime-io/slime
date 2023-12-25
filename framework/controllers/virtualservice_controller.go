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
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model"
)

// VirtualServiceReconciler reconciles a VirtualService object
type VirtualServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Env    *bootstrap.Environment
}

// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices/status,verbs=get;update;patch

func (r *VirtualServiceReconciler) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.WithField("virtualService", req.NamespacedName)
	// Fetch the VirtualService instance
	instance := &networkingv1alpha3.VirtualService{}
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

	istioRev := model.IstioRevFromLabel(instance.Labels)
	if !r.Env.RevInScope(istioRev) {
		log.Debugf("existing virtualService %v istiorev %s but out %s, skip ...",
			req.NamespacedName, istioRev, r.Env.IstioRev())
		return ctrl.Result{}, nil
	}

	// 资源更新
	m := parseDestination(instance)
	log.Debugf("get destination after parse, %+v", m)
	for k, v := range m {
		HostDestinationMapping.Set(k, v)
	}

	return ctrl.Result{}, nil
}

func parseDestination(instance *networkingv1alpha3.VirtualService) map[string][]string {
	ret := make(map[string][]string)

	hosts := make([]string, 0, len(instance.Spec.Hosts))
	copy(hosts, instance.Spec.Hosts)

	dhs := make(map[string]struct{}, 0)
	for _, httpRoute := range instance.Spec.Http {
		for _, route := range httpRoute.Route {
			dhs[route.Destination.Host] = struct{}{}
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
		For(&networkingv1alpha3.VirtualService{}).
		Complete(r)
}
