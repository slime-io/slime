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
	istio "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"slime.io/slime/slime-framework/apis/networking/v1alpha3"
	"slime.io/slime/slime-framework/util"
	microserviceslimeiov1alpha1types "slime.io/slime/slime-modules/plugin/api/v1alpha1"
	microserviceslimeiov1alpha1 "slime.io/slime/slime-modules/plugin/api/v1alpha1/wrapper"
)

// EnvoyPluginReconciler reconciles a EnvoyPlugin object
type EnvoyPluginReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=envoyplugins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=envoyplugins/status,verbs=get;update;patch

func (r *EnvoyPluginReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	// Fetch the EnvoyPlugin instance
	instance := &microserviceslimeiov1alpha1.EnvoyPlugin{}
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
	ef := r.newEnvoyFilterForEnvoyPlugin(instance)
	if ef == nil {
		return reconcile.Result{}, nil
	}

	// 测试需要
	if r.Scheme != nil {
		// Set EnvoyPlugin instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, ef, r.Scheme); err != nil {
			return reconcile.Result{}, nil
		}
	}

	found := &v1alpha3.EnvoyFilter{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: ef.Name, Namespace: ef.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log.Infof("Creating a new EnvoyFilter in %s:%s", ef.Namespace, ef.Name)
		err = r.Client.Create(context.TODO(), ef)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	} else {
		log.Infof("Update a EnvoyFilter in %s:%s", ef.Namespace, ef.Name)
		ef.ResourceVersion = found.ResourceVersion
		err := r.Client.Update(context.TODO(), ef)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *EnvoyPluginReconciler) newEnvoyFilterForEnvoyPlugin(cr *microserviceslimeiov1alpha1.EnvoyPlugin) *v1alpha3.EnvoyFilter {
	pb, err := util.FromJSONMap("slime.microservice.v1alpha1.EnvoyPlugin", cr.Spec)
	if err != nil {
		log.Errorf("unable to convert envoyPlugin to envoyFilter,%+v", err)
		return nil
	}
	envoyFilter := &istio.EnvoyFilter{}
	r.translateEnvoyPlugin(pb.(*microserviceslimeiov1alpha1types.EnvoyPlugin), envoyFilter)
	envoyFilterWrapper := &v1alpha3.EnvoyFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
	}
	if m, err := util.ProtoToMap(envoyFilter); err == nil {
		envoyFilterWrapper.Spec = m
		return envoyFilterWrapper
	}
	return nil
}

func (r *EnvoyPluginReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microserviceslimeiov1alpha1.EnvoyPlugin{}).
		Complete(r)
}
