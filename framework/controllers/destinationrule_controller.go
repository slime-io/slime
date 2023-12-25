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
	"slime.io/slime/framework/util"
)

// DestinationRuleReconciler reconciles a DestinationRule object
type DestinationRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Env    *bootstrap.Environment
}

// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules/status,verbs=get;update;patch

func (r *DestinationRuleReconciler) Reconcile(_ context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.WithField("destinationRule", req.NamespacedName)

	// Fetch the DestinationRule instance
	instance := &networkingv1alpha3.DestinationRule{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO del event not handled. should re-calc the data accordingly
			log.Infof("destinationrule is deleted")
			return reconcile.Result{}, nil
		} else {
			log.Errorf("get destinationRule error, %+v", err)
			return reconcile.Result{}, err
		}
	}

	istioRev := model.IstioRevFromLabel(instance.Labels)
	if !r.Env.RevInScope(istioRev) {
		log.Debugf("existing destinationRule %v istiorev %s but out %s, skip ...",
			req.NamespacedName, istioRev, r.Env.IstioRev())
		return ctrl.Result{}, nil
	}
	log.Infof("get destinationRule, %s", instance.Name)

	// 资源更新
	drHost := util.UnityHost(instance.Spec.Host, instance.Namespace)
	HostSubsetMapping.Set(drHost, instance.Spec.Subsets)

	return ctrl.Result{}, nil
}

func (r *DestinationRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha3.DestinationRule{}).
		Complete(r)
}
