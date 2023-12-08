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

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model"
	"slime.io/slime/modules/plugin/api/config"
	microserviceslimeiov1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1"
)

// EnvoyPluginReconciler reconciles a EnvoyPlugin object
type EnvoyPluginReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Env    *bootstrap.Environment
	Cfg    *config.PluginModule
}

// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=envoyplugins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=envoyplugins/status,verbs=get;update;patch

func (r *EnvoyPluginReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	EnvoypluginReconciles.Increment()
	// Fetch the EnvoyPlugin instance
	instance := &microserviceslimeiov1alpha1.EnvoyPlugin{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		} else {
			EnvoypluginReconcilesFailed.Increment()
			return reconcile.Result{}, err
		}
	}

	istioRev := model.IstioRevFromLabel(instance.Labels)
	if !r.Env.RevInScope(istioRev) {
		log.Debugf("existing envoyplugin %v istiorev %s but out %s, skip ...",
			req.NamespacedName, istioRev, r.Env.IstioRev())
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
	model.PatchObjectMeta(&ef.ObjectMeta, &instance.ObjectMeta)
	model.PatchIstioRevLabel(&ef.Labels, istioRev)

	found := &v1alpha3.EnvoyFilter{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: ef.Name, Namespace: ef.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			err = nil
			found = nil
		} else {
			EnvoypluginReconcilesFailed.Increment()
			return reconcile.Result{}, err
		}
	}

	if found == nil {
		log.Infof("Creating a new EnvoyFilter in %s:%s", ef.Namespace, ef.Name)
		err = r.Client.Create(ctx, ef)
		if err != nil {
			EnvoypluginReconcilesFailed.Increment()
			return reconcile.Result{}, err
		}
		EnvoyfilterCreations.With(resourceName.Value("envoyplugin")).Increment()
	} else if foundRev := model.IstioRevFromLabel(found.Labels); !r.Env.RevInScope(foundRev) {
		log.Debugf("existed envoyfilter %v istioRev %s but our rev %s, skip updating to %+v",
			req.NamespacedName, found, r.Env.IstioRev(), ef)
	} else {
		log.Infof("Update a EnvoyFilter in %s:%s", ef.Namespace, ef.Name)
		ef.ResourceVersion = found.ResourceVersion
		err := r.Client.Update(ctx, ef)
		if err != nil {
			EnvoypluginReconcilesFailed.Increment()
			return reconcile.Result{}, err
		}
		EnvoyfilterRefreshes.With(resourceName.Value("envoyplugin")).Increment()
	}

	return ctrl.Result{}, nil
}

func (r *EnvoyPluginReconciler) newEnvoyFilterForEnvoyPlugin(cr *microserviceslimeiov1alpha1.EnvoyPlugin) *v1alpha3.EnvoyFilter {
	out := r.translateEnvoyPlugin(cr)
	envoyFilterWrapper, err := translateOutputToEnvoyFilterWrapper(out)
	if err != nil {
		log.Errorf("translateOutputToEnvoyFilterWrapper for envoyfilter %s/%s met err %v", cr.Namespace, cr.Name, err)
	}
	envoyFilterWrapper.Name, envoyFilterWrapper.Namespace = cr.Name, cr.Namespace

	return envoyFilterWrapper
}

func (r *EnvoyPluginReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microserviceslimeiov1alpha1.EnvoyPlugin{}).
		Complete(r)
}
