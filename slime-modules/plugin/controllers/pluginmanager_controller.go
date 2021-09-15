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
	"slime.io/slime/slime-framework/bootstrap"
	"slime.io/slime/slime-framework/util"
	microserviceslimeiov1alpha1types "slime.io/slime/slime-modules/plugin/api/v1alpha1"
	"slime.io/slime/slime-modules/plugin/api/v1alpha1/wrapper"
	"slime.io/slime/slime-modules/plugin/controllers/wasm"

	microserviceslimeiov1alpha1 "slime.io/slime/slime-modules/plugin/api/v1alpha1/wrapper"
)

// PluginManagerReconciler reconciles a PluginManager object
type PluginManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	wasm wasm.Getter
	env  *bootstrap.Environment
}

// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=pluginmanagers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=pluginmanagers/status,verbs=get;update;patch

func (r *PluginManagerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	// your logic here
	// Fetch the PluginManager instance
	instance := &wrapper.PluginManager{}
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
	ef := r.newPluginManagerForEnvoyPlugin(instance)
	if ef == nil {
		// 由于配置错误导致的，因此直接返回nil，避免reconcile重试
		return reconcile.Result{}, nil
	}
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

func (r *PluginManagerReconciler) newPluginManagerForEnvoyPlugin(cr *wrapper.PluginManager) *v1alpha3.EnvoyFilter {
	pb, err := util.FromJSONMap("slime.microservice.v1alpha1.PluginManager", cr.Spec)
	if err != nil {
		log.Errorf("unable to convert pluginManager to envoyFilter, %+v", err)
		return nil
	}

	envoyFilter := &istio.EnvoyFilter{}
	r.translatePluginManager(pb.(*microserviceslimeiov1alpha1types.PluginManager), envoyFilter)
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

func (r *PluginManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microserviceslimeiov1alpha1.PluginManager{}).
		Complete(r)
}
