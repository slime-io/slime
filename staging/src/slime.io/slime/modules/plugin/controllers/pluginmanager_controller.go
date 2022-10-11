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
	istio "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sync"

	"slime.io/slime/framework/apis/networking/v1alpha3"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model"
	"slime.io/slime/framework/util"
	microserviceslimeiov1alpha1types "slime.io/slime/modules/plugin/api/v1alpha1"
	"slime.io/slime/modules/plugin/api/v1alpha1/wrapper"
	microserviceslimeiov1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1/wrapper"
)

// PluginManagerReconciler reconciles a PluginManager object
type PluginManagerReconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	kubeInformer informers.SharedInformerFactory

	credController *CredentialsController
	env            bootstrap.Environment

	mut                  sync.RWMutex
	secretWatchers       map[types.NamespacedName]map[types.NamespacedName]struct{}
	changeSecrets        map[types.NamespacedName]struct{}
	changeSecretNotifyCh chan struct{}
}

func NewPluginManagerReconciler(env bootstrap.Environment, client client.Client, scheme *runtime.Scheme) *PluginManagerReconciler {
	return &PluginManagerReconciler{
		client:               client,
		scheme:               scheme,
		env:                  env,
		secretWatchers:       map[types.NamespacedName]map[types.NamespacedName]struct{}{},
		changeSecrets:        map[types.NamespacedName]struct{}{},
		changeSecretNotifyCh: make(chan struct{}, 1),
		kubeInformer:         informers.NewSharedInformerFactory(env.K8SClient, 0),
	}
}

// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=pluginmanagers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io.my.domain,resources=pluginmanagers/status,verbs=get;update;patch

func (r *PluginManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.reconcile(ctx, req.NamespacedName)
}

func (r *PluginManagerReconciler) reconcile(ctx context.Context, nn types.NamespacedName) (ctrl.Result, error) {
	// Fetch the PluginManager instance
	instance := &wrapper.PluginManager{}
	err := r.client.Get(ctx, nn, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO del relevant resource
			return reconcile.Result{}, nil
		} else {
			return reconcile.Result{}, err
		}
	}

	istioRev := model.IstioRevFromLabel(instance.Labels)
	if !r.env.RevInScope(istioRev) {
		log.Debugf("existing pluginmanager %v istiorev %s but out %s, skip ...",
			nn, istioRev, r.env.IstioRev())
		return reconcile.Result{}, nil
	}

	// 资源更新
	pluginManager := &microserviceslimeiov1alpha1types.PluginManager{}
	if err = util.FromJSONMapToMessage(instance.Spec, pluginManager); err != nil {
		log.Errorf("unable to convert pluginManager to envoyFilter, %+v", err)
		// 由于配置错误导致的，因此直接返回nil，避免reconcile重试
		return reconcile.Result{}, nil
	}

	watchSecrets := getPluginManagerWatchSecrets(nn.Namespace, pluginManager)
	r.updateWatchSecrets(nn, watchSecrets) // XXX concurrent...

	ef := r.newPluginManagerForEnvoyPlugin(instance, pluginManager)
	if ef == nil {
		// 由于配置错误导致的，因此直接返回nil，避免reconcile重试
		return reconcile.Result{}, nil
	}
	if r.scheme != nil {
		// Set EnvoyPlugin instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, ef, r.scheme); err != nil {
			return reconcile.Result{}, nil
		}
	}
	model.PatchObjectMeta(&ef.ObjectMeta, &instance.ObjectMeta)
	model.PatchIstioRevLabel(&ef.Labels, istioRev)

	found := &v1alpha3.EnvoyFilter{}
	nsName := types.NamespacedName{Name: ef.Name, Namespace: ef.Namespace}
	err = r.client.Get(ctx, nsName, found)
	if err != nil {
		if errors.IsNotFound(err) {
			found = nil
		}
	}

	if found == nil {
		log.Infof("Creating a new EnvoyFilter in %s:%s", ef.Namespace, ef.Name)
		err := r.client.Create(ctx, ef)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if model.IstioRevFromLabel(found.Labels) != istioRev {
		log.Debugf("existing envoyfilter %v istioRev %s but our %s, skip ...", nsName, model.IstioRevFromLabel(found.Labels), istioRev)
		return reconcile.Result{}, nil
	} else {
		log.Infof("Update a EnvoyFilter in %v", nsName)
		ef.ResourceVersion = found.ResourceVersion
		err := r.client.Update(ctx, ef)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *PluginManagerReconciler) newPluginManagerForEnvoyPlugin(cr *wrapper.PluginManager, pluginManager *microserviceslimeiov1alpha1types.PluginManager) *v1alpha3.EnvoyFilter {
	envoyFilter := &istio.EnvoyFilter{}
	r.translatePluginManager(cr.ObjectMeta, pluginManager, envoyFilter)

	m, err := util.ProtoToMap(envoyFilter)
	if err != nil {
		log.Errorf("ProtoToMap for envoyfilter %s/%s met err %v", cr.Namespace, cr.Name, err)
		return nil
	}

	return &v1alpha3.EnvoyFilter{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name,
			Namespace: cr.Namespace,
		},
		Spec: m,
	}
}

func (r *PluginManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//_ = r.kubeInformer.Core().V1().Secrets().Informer()

	r.credController = NewCredentialsController(r.kubeInformer)
	r.credController.AddEventHandler(func(name string, namespace string) {
		r.notifySecretChange(types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		})
	})

	r.kubeInformer.Start(nil)
	r.kubeInformer.WaitForCacheSync(nil)

	go r.handleSecretChange()

	return ctrl.NewControllerManagedBy(mgr).
		For(&microserviceslimeiov1alpha1.PluginManager{}).
		Complete(r)
}

func (r *PluginManagerReconciler) notifySecretChange(nn types.NamespacedName) {
	r.mut.Lock()
	r.changeSecrets[nn] = struct{}{}
	r.mut.Unlock()

	select {
	case r.changeSecretNotifyCh <- struct{}{}:
	default:
	}
}

func (r *PluginManagerReconciler) handleSecretChange() {
	for {
		select {
		case <-r.changeSecretNotifyCh:
		}

		var (
			resourceToReconcile map[types.NamespacedName]struct{}
			changedSecrets      map[types.NamespacedName]struct{}
		)
		r.mut.Lock()
		changedSecrets = r.changeSecrets
		if len(changedSecrets) > 0 {
			r.changeSecrets = map[types.NamespacedName]struct{}{}
		}
		r.mut.Unlock()

		if len(changedSecrets) == 0 {
			continue
		}

		log.Infof("handle changed secrets %+v", changedSecrets)
		r.mut.RLock()
		for secretNn := range changedSecrets {
			for w := range r.secretWatchers[secretNn] {
				resourceToReconcile[w] = struct{}{}
			}
		}
		r.mut.RUnlock()

		for nn := range resourceToReconcile {
			_, err := r.reconcile(context.Background(), nn)
			if err != nil {
				log.Errorf("handleSecretChange reconcile %v met err %v", nn, err)
			}
		}
	}
}

func (r *PluginManagerReconciler) updateWatchSecrets(nn types.NamespacedName, secrets map[types.NamespacedName]struct{}) {
	r.mut.Lock()
	defer r.mut.Unlock()

	for secretNn, watchers := range r.secretWatchers {
		if _, ok := secrets[secretNn]; ok {
			watchers[nn] = struct{}{}
		} else {
			delete(watchers, nn)
		}
	}
	for secretNn := range secrets {
		if _, ok := r.secretWatchers[secretNn]; !ok {
			r.secretWatchers[secretNn] = map[types.NamespacedName]struct{}{nn: {}}
		}
	}
}

func getPluginManagerWatchSecrets(ns string, in *microserviceslimeiov1alpha1types.PluginManager) map[types.NamespacedName]struct{} {
	ret := map[types.NamespacedName]struct{}{}
	for _, p := range in.GetPlugin() {
		wasm := p.GetWasm()
		if wasm == nil {
			continue
		}
		if secret := wasm.GetImagePullSecretName(); secret != "" {
			ret[types.NamespacedName{
				Namespace: ns,
				Name:      secret,
			}] = struct{}{}
		}
	}
	return ret
}
