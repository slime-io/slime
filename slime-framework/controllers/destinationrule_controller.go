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
	"fmt"
	"github.com/go-logr/logr"

	istionetworking "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	networkingistioiov1alpha3 "slime.io/slime/slime-framework/apis/networking/v1alpha3"
	"slime.io/slime/slime-framework/util"
)

// DestinationRuleReconciler reconciles a DestinationRule object
type DestinationRuleReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules/status,verbs=get;update;patch

func (r *DestinationRuleReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("destinationrule", req.NamespacedName)

	// Fetch the DestinationRule instance
	instance := &networkingistioiov1alpha3.DestinationRule{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		r.Log.Error(err, fmt.Sprintf("get destinationrule %+v abnormal, unknown condition",req.NamespacedName))
		return reconcile.Result{}, err
	}

	r.Log.Info(fmt.Sprintf("get destinationrule %+v",*instance))

	// 资源更新
	pb, err := util.FromJSONMap("istio.networking.v1alpha3.DestinationRule", instance.Spec)
	if err != nil {
		return reconcile.Result{}, nil
	}
	if dr, ok := pb.(*istionetworking.DestinationRule); ok {
		drHost := util.UnityHost(dr.Host, instance.Namespace)
		HostSubsetMapping.Set(drHost, dr.Subsets)
	}

	return ctrl.Result{}, nil
}

func DoUpdate(i v1.Object, args ...interface{}) error {
	if instance, ok := i.(*networkingistioiov1alpha3.DestinationRule); ok {
		pb, err := util.FromJSONMap("istio.networking.v1alpha3.DestinationRule", instance.Spec)
		if err != nil {
			return err
		}
		if dr, ok := pb.(*istionetworking.DestinationRule); ok {
			drHost := util.UnityHost(dr.Host, instance.Namespace)
			HostSubsetMapping.Set(drHost, dr.Subsets)
		}
	}
	return nil
}

func (r *DestinationRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingistioiov1alpha3.DestinationRule{}).
		Complete(r)
}
