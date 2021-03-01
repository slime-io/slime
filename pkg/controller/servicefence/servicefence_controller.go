/*
* @Author: yangdihang
* @Date: 2020/5/21
 */

package servicefence

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"slime.io/slime/pkg/apis/config/v1alpha1"
	microservicev1alpha1 "slime.io/slime/pkg/apis/microservice/v1alpha1"
	"slime.io/slime/pkg/apis/networking/v1alpha3"
	"slime.io/slime/pkg/bootstrap"
	controller2 "slime.io/slime/pkg/controller"
	"slime.io/slime/pkg/controller/virtualservice"
	event_source "slime.io/slime/pkg/model/source"
	"slime.io/slime/pkg/model/source/aggregate"
	"slime.io/slime/pkg/model/source/k8s"
	"slime.io/slime/pkg/util"

	istio "istio.io/api/networking/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_servicefence")
var config *v1alpha1.Config

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ServiceFence Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, environment *bootstrap.Environment) error {
	r := newReconciler(mgr, environment).(*ReconcileServiceFence)
	virtualservice.HostDestinationMapping.Subscribe(r.Subscribe)
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, env *bootstrap.Environment) reconcile.Reconciler {
	if env.Config.Metric != nil {
		eventChan := make(chan event_source.Event)
		src := &aggregate.Source{}
		if ms, err := k8s.NewMetricSource(eventChan, env); err != nil {
			log.Error(err, "failed to create slime-metric")
		} else {
			src.Sources = append(src.Sources, ms)

			r := &ReconcileServiceFence{
				client:    mgr.GetClient(),
				scheme:    mgr.GetScheme(),
				env:       env,
				eventChan: eventChan,
				source:    src,
			}
			r.source.Start(env.Stop)
			r.WatchSource(env.Stop)
			return r
		}
	}
	return &ReconcileServiceFence{client: mgr.GetClient(), scheme: mgr.GetScheme(), env: env}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("servicefence-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ServiceFence
	err = c.Watch(&source.Kind{Type: &microservicev1alpha1.ServiceFence{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileServiceFence implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileServiceFence{}

// ReconcileServiceFence reconciles a ServiceFence object
type ReconcileServiceFence struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme

	env       *bootstrap.Environment
	eventChan chan event_source.Event
	source    event_source.Source
}

// Reconcile reads that state of the cluster for a ServiceFence object and makes changes based on the state read
// and what is in the ServiceFence.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileServiceFence) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ServiceFence")

	// Fetch the ServiceFence instance
	instance := &microservicev1alpha1.ServiceFence{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)

	// 异常分支
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	// 资源删除
	if err != nil && errors.IsNotFound(err) {
		for _, f := range controller2.DeleteHook[controller2.ServiceFence] {
			if err := f(request, r); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// 资源更新
	for _, f := range controller2.UpdateHook[controller2.ServiceFence] {
		if err := f(instance, r); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func DoUpdate(i metav1.Object, args ...interface{}) error {
	if len(args) == 0 {
		log.Error(nil, "servicefence doUpdate方法参数不足")
		return nil
	}
	if this, ok := args[0].(*ReconcileServiceFence); !ok {
		log.Error(nil, "servicefence doUpdate方法参数不足")
	} else {
		if instance, ok := i.(*microservicev1alpha1.ServiceFence); !ok {
			log.Error(nil, "servicefence doRemove方法第一参数需为自身")
		} else {
			diff := this.updateVisitedHostStatus(instance)
			this.recordVisitor(instance, diff)
			if instance.Spec.Enable {
				if this.source != nil {
					this.source.WatchAdd(types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace})
				}
				sidecar, err := newSidecar(instance, this.env)
				if err != nil {
					log.Error(err, "servicefence生产sidecar的过程中发生错误")
					return err
				}
				if sidecar == nil {
					return nil
				}
				// Set VisitedHost instance as the owner and controller
				if err := controllerutil.SetControllerReference(instance, sidecar, this.scheme); err != nil {
					log.Error(err, "servicefence为sidecar添加ownerReference的过程中发生错误")
					return err
				}
				// Check if this Pod already exists
				found := &v1alpha3.Sidecar{}
				err = this.client.Get(context.TODO(), types.NamespacedName{Name: sidecar.Name, Namespace: sidecar.Namespace}, found)
				if err != nil && errors.IsNotFound(err) {
					log.Info("Creating a new Sidecar", "Sidecar.Namespace", sidecar.Namespace, "Sidecar.Name", sidecar.Name)
					err = this.client.Create(context.TODO(), sidecar)
					if err != nil {
						return err
					}
				} else if err == nil {
					if !reflect.DeepEqual(found.Spec, sidecar.Spec) {
						log.Info("Update a  Sidecar", "Sidecar.Namespace", sidecar.Namespace, "Sidecar.Name", sidecar.Name)
						sidecar.ResourceVersion = found.ResourceVersion
						err = this.client.Update(context.TODO(), sidecar)
						if err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func (r *ReconcileServiceFence) recordVisitor(host *microservicev1alpha1.ServiceFence, diff Diff) {
	for _, k := range diff.Added {
		vih := r.prepare(host, k)
		if vih == nil {
			continue
		}
		vih.Status.Visitor[host.Namespace+"/"+host.Name] = true
		_ = r.client.Status().Update(context.TODO(), vih)
	}

	for _, k := range diff.Deleted {
		vih := r.prepare(host, k)
		if vih == nil {
			continue
		}
		delete(vih.Status.Visitor, host.Namespace+"/"+host.Name)
		_ = r.client.Status().Update(context.TODO(), vih)
	}
}

func (r *ReconcileServiceFence) prepare(host *microservicev1alpha1.ServiceFence, n string) *microservicev1alpha1.ServiceFence {
	loc := parseHost(host.Namespace, n)
	if loc == nil {
		return nil
	}
	svc := &v1.Service{}
	if err := r.client.Get(context.TODO(), *loc, svc); err != nil {
		return nil
	}
	vih := &microservicev1alpha1.ServiceFence{}
retry:
	if err := r.client.Get(context.TODO(), *loc, vih); err != nil {
		if errors.IsNotFound(err) {
			vih.Name = loc.Name
			vih.Namespace = loc.Namespace
			if err = r.client.Create(context.TODO(), vih); err != nil {
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

func (r *ReconcileServiceFence) updateVisitedHostStatus(host *microservicev1alpha1.ServiceFence) Diff {
	domains := make(map[string]*microservicev1alpha1.Destinations)
	now := time.Now().Unix()
	for k, v := range host.Spec.Host {
		allHost := []string{k}
		if hs := getDestination(k); len(hs) > 0 {
			allHost = append(allHost, hs...)
		}
		var status microservicev1alpha1.Destinations_Status
		if v.Stable != nil {
			status = microservicev1alpha1.Destinations_ACTIVE
		} else if v.Deadline != nil {
			if now-v.Deadline.Expire.Seconds > 0 {
				status = microservicev1alpha1.Destinations_EXPIRE
			} else {
				status = microservicev1alpha1.Destinations_ACTIVE
			}
		} else if v.Auto != nil {
			if v.RecentlyCalled == nil {
				status = microservicev1alpha1.Destinations_ACTIVE
			} else {
				if now-v.RecentlyCalled.Seconds > v.Auto.Duration.Seconds {
					status = microservicev1alpha1.Destinations_EXPIRE
				} else {
					status = microservicev1alpha1.Destinations_ACTIVE
				}
			}
		}
		domains[k] = &microservicev1alpha1.Destinations{
			Hosts:  allHost,
			Status: status,
		}
	}

	for mk, _ := range host.Status.MetricStatus {
		mk = strings.Trim(mk, "{}")
		if strings.HasPrefix(mk, "destination_service") {
			ss := strings.Split(mk, "\"")
			if len(ss) != 3 {
				continue
			} else {
				k := ss[1]
				ks := strings.Split(k, ".")
				var unityHost string
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
				domains[k] = &microservicev1alpha1.Destinations{
					Hosts:  allHost,
					Status: microservicev1alpha1.Destinations_ACTIVE,
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
	_ = r.client.Status().Update(context.TODO(), host)
	return delta
}

func newSidecar(vhost *microservicev1alpha1.ServiceFence, env *bootstrap.Environment) (*v1alpha3.Sidecar, error) {
	host := make([]string, 0)
	if !vhost.Spec.Enable {
		return nil, nil
	}
	for _, v := range vhost.Status.Domains {
		if v.Status == microservicev1alpha1.Destinations_ACTIVE {
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
				//Bind:  "0.0.0.0",
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

func (r *ReconcileServiceFence) Subscribe(host string, destination interface{}) {
	if svc, namespace, ok := util.IsK8SService(host); ok {
		vih := &microservicev1alpha1.ServiceFence{}
		if err := r.client.Get(context.TODO(), types.NamespacedName{Name: svc, Namespace: namespace}, vih); err != nil {
			return
		}
		for k, _ := range vih.Status.Visitor {
			if i := strings.Index(k, "/"); i != -1 {
				visitorVih := &microservicev1alpha1.ServiceFence{}
				visitorNamespace := k[:i]
				visitorName := k[i+1:]
				err := r.client.Get(context.TODO(), types.NamespacedName{Name: visitorName, Namespace: visitorNamespace}, visitorVih)
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
				if err := controllerutil.SetControllerReference(visitorVih, sidecarScope, r.scheme); err != nil {
					return
				}

				// Check if this Pod already exists
				found := &v1alpha3.Sidecar{}
				err = r.client.Get(context.TODO(), types.NamespacedName{Name: sidecarScope.Name, Namespace: sidecarScope.Namespace}, found)
				if err != nil && errors.IsNotFound(err) {
					err = r.client.Create(context.TODO(), sidecarScope)
					return
				} else if err != nil {
					if !reflect.DeepEqual(found.Spec, sidecarScope.Spec) {
						sidecarScope.ResourceVersion = found.ResourceVersion
						err = r.client.Update(context.TODO(), sidecarScope)
					}
					return
				}
			}
		}
	}
	return
}

func getDestination(k string) []string {
	if i := virtualservice.HostDestinationMapping.Get(k); i != nil {
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
