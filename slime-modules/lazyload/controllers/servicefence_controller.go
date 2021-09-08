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
	"reflect"
	"strings"
	"sync"
	"time"

	istio "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"slime.io/slime/slime-framework/apis/networking/v1alpha3"
	"slime.io/slime/slime-framework/bootstrap"
	"slime.io/slime/slime-framework/controllers"
	event_source "slime.io/slime/slime-framework/model/source"
	"slime.io/slime/slime-framework/model/source/aggregate"
	"slime.io/slime/slime-framework/model/source/k8s"
	"slime.io/slime/slime-framework/util"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lazyloadv1alpha1 "slime.io/slime/slime-modules/lazyload/api/v1alpha1"
)

// ServicefenceReconciler reconciles a Servicefence object
type ServicefenceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	env       *bootstrap.Environment
	eventChan chan event_source.Event
	source    event_source.Source

	reconcileLock sync.Mutex

	staleNamespaces   map[string]bool
	enabledNamespaces map[string]bool
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, env *bootstrap.Environment) *ServicefenceReconciler {
	r := &ServicefenceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("ServiceFence"),
		env:    env,

		staleNamespaces:   map[string]bool{},
		enabledNamespaces: map[string]bool{},
	}

	if env.Config.Metric != nil {
		eventChan := make(chan event_source.Event)
		src := &aggregate.Source{}
		if ms, err := k8s.NewMetricSource(eventChan, env); err != nil {
			ctrl.Log.Error(err, "failed to create slime-metric")
		} else {
			src.Sources = append(src.Sources, ms)
			r.eventChan = eventChan
			r.source = src

			r.source.Start(env.Stop)
			r.WatchSource(env.Stop)
		}
	}
	return r
}

// +kubebuilder:rbac:groups=microservice.slime.io,resources=servicefences,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io,resources=servicefences/status,verbs=get;update;patch

func (r *ServicefenceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("serviceFence", req.NamespacedName)

	// Fetch the ServiceFence instance
	instance := &lazyloadv1alpha1.ServiceFence{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("serviceFence is deleted")
			r.source.WatchRemove(req.NamespacedName)
			return reconcile.Result{}, nil
		} else {
			r.Log.Error(err, "get serviceFence error")
			return reconcile.Result{}, err
		}
	}
	r.Log.Info("get serviceFence", "sf", instance)

	// 资源更新
	diff := r.updateVisitedHostStatus(instance)
	r.recordVisitor(instance, diff)
	if instance.Spec.Enable {
		if r.source != nil {
			r.source.WatchAdd(types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace})
		}
		err = r.refreshSidecar(instance)
	}

	return ctrl.Result{}, err
}

func (r *ServicefenceReconciler) refreshSidecar(instance *lazyloadv1alpha1.ServiceFence) error {
	sidecar, err := newSidecar(instance, r.env)
	if err != nil {
		r.Log.Error(err, "servicefence生产sidecar的过程中发生错误")
		return err
	}
	if sidecar == nil {
		return nil
	}
	// Set VisitedHost instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, sidecar, r.Scheme); err != nil {
		r.Log.Error(err, "servicefence为sidecar添加ownerReference的过程中发生错误")
		return err
	}
	// Check if this Pod already exists
	found := &v1alpha3.Sidecar{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: sidecar.Name, Namespace: sidecar.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		r.Log.Info("Creating a new Sidecar", "Sidecar.Namespace", sidecar.Namespace, "Sidecar.Name", sidecar.Name)
		err = r.Client.Create(context.TODO(), sidecar)
		if err != nil {
			return err
		}
	} else if err == nil {
		if !reflect.DeepEqual(found.Spec, sidecar.Spec) {
			r.Log.Info("Update a  Sidecar", "Sidecar.Namespace", sidecar.Namespace, "Sidecar.Name", sidecar.Name)
			sidecar.ResourceVersion = found.ResourceVersion
			err = r.Client.Update(context.TODO(), sidecar)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ServicefenceReconciler) recordVisitor(host *lazyloadv1alpha1.ServiceFence, diff Diff) {
	for _, k := range diff.Added {
		vih := r.prepare(host, k)
		if vih == nil {
			continue
		}
		vih.Status.Visitor[host.Namespace+"/"+host.Name] = true
		_ = r.Client.Status().Update(context.TODO(), vih)
	}

	for _, k := range diff.Deleted {
		vih := r.prepare(host, k)
		if vih == nil {
			continue
		}
		delete(vih.Status.Visitor, host.Namespace+"/"+host.Name)
		_ = r.Client.Status().Update(context.TODO(), vih)
	}
}

func (r *ServicefenceReconciler) prepare(host *lazyloadv1alpha1.ServiceFence, n string) *lazyloadv1alpha1.ServiceFence {
	loc := parseHost(host.Namespace, n)
	if loc == nil {
		return nil
	}
	svc := &corev1.Service{}
	if err := r.Client.Get(context.TODO(), *loc, svc); err != nil {
		return nil
	}
	vih := &lazyloadv1alpha1.ServiceFence{}
retry:
	if err := r.Client.Get(context.TODO(), *loc, vih); err != nil {
		if errors.IsNotFound(err) {
			vih.Name = loc.Name
			vih.Namespace = loc.Namespace
			if err = r.Client.Create(context.TODO(), vih); err != nil {
				goto retry
			}
		}
	}

	if vih.Status.Visitor == nil {
		vih.Status.Visitor = make(map[string]bool)
	}
	return vih
}

func parseHost(locNs, h string) *types.NamespacedName {
	s := strings.Split(h, ".")
	if len(s) == 5 || len(s) == 2 {
		return &types.NamespacedName{
			Namespace: s[1],
			Name:      s[0],
		}
	}
	if len(s) == 1 {
		return &types.NamespacedName{
			Namespace: locNs,
			Name:      s[0],
		}
	}
	return nil
}

func (r *ServicefenceReconciler) updateVisitedHostStatus(host *lazyloadv1alpha1.ServiceFence) Diff {
	domains := make(map[string]*lazyloadv1alpha1.Destinations)
	now := time.Now().Unix()
	for k, v := range host.Spec.Host {
		allHost := []string{k}
		if hs := getDestination(k); len(hs) > 0 {
			allHost = append(allHost, hs...)
		}
		var status lazyloadv1alpha1.Destinations_Status
		if v.Stable != nil {
			status = lazyloadv1alpha1.Destinations_ACTIVE
		} else if v.Deadline != nil {
			if now-v.Deadline.Expire.Seconds > 0 {
				status = lazyloadv1alpha1.Destinations_EXPIRE
			} else {
				status = lazyloadv1alpha1.Destinations_ACTIVE
			}
		} else if v.Auto != nil {
			if v.RecentlyCalled == nil {
				status = lazyloadv1alpha1.Destinations_ACTIVE
			} else {
				if now-v.RecentlyCalled.Seconds > v.Auto.Duration.Seconds {
					status = lazyloadv1alpha1.Destinations_EXPIRE
				} else {
					status = lazyloadv1alpha1.Destinations_ACTIVE
				}
			}
		}
		domains[k] = &lazyloadv1alpha1.Destinations{
			Hosts:  allHost,
			Status: status,
		}
	}

	for mk := range host.Status.MetricStatus {
		mk = strings.Trim(mk, "{}")
		if strings.HasPrefix(mk, "destination_service") {
			ss := strings.Split(mk, "\"")
			if len(ss) != 3 {
				continue
			} else {
				k := ss[1]
				ks := strings.Split(k, ".")
				unityHost := k
				if len(ks) == 1 {
					unityHost = fmt.Sprintf("%s.%s.svc.cluster.local", ks[0], host.Namespace)
				} else if len(ks) == 2 {
					unityHost = fmt.Sprintf("%s.%s.svc.cluster.local", ks[0], ks[1])
				}
				if !isValidHost(unityHost) {
					continue
				}
				if domains[unityHost] != nil {
					continue
				}
				allHost := []string{unityHost}
				if hs := getDestination(unityHost); len(hs) > 0 {
					allHost = append(allHost, hs...)
				}
				domains[k] = &lazyloadv1alpha1.Destinations{
					Hosts:  allHost,
					Status: lazyloadv1alpha1.Destinations_ACTIVE,
				}
			}
		}
	}
	delta := Diff{
		Deleted: make([]string, 0),
		Added:   make([]string, 0),
	}
	for k := range host.Status.Domains {
		if _, ok := domains[k]; !ok {
			delta.Deleted = append(delta.Deleted, k)
		}
	}
	for k := range domains {
		if _, ok := host.Status.Domains[k]; !ok {
			delta.Added = append(delta.Added, k)
		}
	}
	host.Status.Domains = domains
	_ = r.Client.Status().Update(context.TODO(), host)
	return delta
}

func newSidecar(vhost *lazyloadv1alpha1.ServiceFence, env *bootstrap.Environment) (*v1alpha3.Sidecar, error) {
	host := make([]string, 0)
	if !vhost.Spec.Enable {
		return nil, nil
	}
	for _, v := range vhost.Status.Domains {
		if v.Status == lazyloadv1alpha1.Destinations_ACTIVE {
			for _, h := range v.Hosts {
				host = append(host, "*/"+h)
			}
		}
	}
	// 需要加入一条根namespace的策略
	host = append(host, env.Config.Global.IstioNamespace+"/*")
	host = append(host, env.Config.Global.SlimeNamespace+"/*")
	host = append(host, fmt.Sprintf("*/global-sidecar.%s.svc.cluster.local", vhost.Namespace))
	sidecar := &istio.Sidecar{
		WorkloadSelector: &istio.WorkloadSelector{
			Labels: map[string]string{env.Config.Global.Service: vhost.Name},
		},
		Egress: []*istio.IstioEgressListener{
			{
				// Bind:  "0.0.0.0",
				Hosts: host,
			},
		},
	}
	if spec, err := util.ProtoToMap(sidecar); err == nil {
		ret := &v1alpha3.Sidecar{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vhost.Name,
				Namespace: vhost.Namespace,
			},
			Spec: spec,
		}
		return ret, nil
	} else {
		return nil, err
	}
}

func (r *ServicefenceReconciler) Subscribe(host string, destination interface{}) {
	if svc, namespace, ok := util.IsK8SService(host); ok {
		vih := &lazyloadv1alpha1.ServiceFence{}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: svc, Namespace: namespace}, vih); err != nil {
			return
		}
		for k := range vih.Status.Visitor {
			if i := strings.Index(k, "/"); i != -1 {
				visitorVih := &lazyloadv1alpha1.ServiceFence{}
				visitorNamespace := k[:i]
				visitorName := k[i+1:]
				err := r.Client.Get(context.TODO(), types.NamespacedName{Name: visitorName, Namespace: visitorNamespace}, visitorVih)
				if err != nil {
					return
				}
				r.updateVisitedHostStatus(visitorVih)
				sidecarScope, err := newSidecar(visitorVih, r.env)
				if sidecarScope == nil {
					continue
				}
				if err != nil {
					return
				}
				// Set VisitedHost instance as the owner and controller
				if err := controllerutil.SetControllerReference(visitorVih, sidecarScope, r.Scheme); err != nil {
					return
				}

				// Check if this Pod already exists
				found := &v1alpha3.Sidecar{}
				err = r.Client.Get(context.TODO(), types.NamespacedName{Name: sidecarScope.Name, Namespace: sidecarScope.Namespace}, found)
				if err != nil && errors.IsNotFound(err) {
					err = r.Client.Create(context.TODO(), sidecarScope)
					return
				} else if err != nil {
					if !reflect.DeepEqual(found.Spec, sidecarScope.Spec) {
						sidecarScope.ResourceVersion = found.ResourceVersion
						err = r.Client.Update(context.TODO(), sidecarScope)
					}
					return
				}
			}
		}
	}
	return
}

func getDestination(k string) []string {
	if i := controllers.HostDestinationMapping.Get(k); i != nil {
		if hs, ok := i.([]string); ok {
			return hs
		}
	}
	return nil
}

// TODO: More rigorous verification
func isValidHost(h string) bool {
	if strings.Contains(h, "global-sidecar") ||
		strings.Contains(h, ":") ||
		strings.Contains(h, "unknown") {
		return false
	}
	return true
}

func (r *ServicefenceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lazyloadv1alpha1.ServiceFence{}).
		Complete(r)
}
