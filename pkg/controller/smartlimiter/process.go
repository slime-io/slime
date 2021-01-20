/*
* @Author: yangdihang
* @Date: 2020/11/19
 */

package smartlimiter

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	config "yun.netease.com/slime/pkg/apis/config/v1alpha1"
	microservicev1alpha1 "yun.netease.com/slime/pkg/apis/microservice/v1alpha1"
	"yun.netease.com/slime/pkg/apis/networking/v1alpha3"
	"yun.netease.com/slime/pkg/model"
	event_source "yun.netease.com/slime/pkg/model/source"
	"yun.netease.com/slime/pkg/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileSmartLimiter) getMaterial(loc types.NamespacedName) map[string]string {
	if i, ok := r.metricInfo.Get(loc.Namespace + "/" + loc.Name); ok {
		if ep, ok := i.(*model.Endpoints); ok {
			return util.CopyMap(ep.Info)
		}
	}
	return nil
}

func refreshEnvoyFilter(instance *microservicev1alpha1.SmartLimiter, r *ReconcileSmartLimiter, obj *v1alpha3.EnvoyFilter) (reconcile.Result, error) {
	name := obj.GetName()
	namespace := obj.GetNamespace()
	if err := controllerutil.SetControllerReference(instance, obj, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	found := &v1alpha3.EnvoyFilter{}

	err := r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, found)

	// Create
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new EnvoyFilter", "EnvoyFilter.Namespace", namespace, "EnvoyFilter.Name", name)
		err = r.client.Create(context.TODO(), obj)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// TODO: 判断是否需要更新
	// Update
	if !reflect.DeepEqual(found.Spec, obj.Spec) {
		log.Info("Update a new EnvoyFilter", "EnvoyFilter.Namespace", namespace, "EnvoyFilter.Name", name)
		obj.ResourceVersion = found.ResourceVersion
		err = r.client.Update(context.TODO(), obj)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Pod created successfully - don't requeue
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileSmartLimiter) WatchSource(stop <-chan struct{}) {
	go func() {
		for {
			select {
			case <-stop:
				return
			case e := <-r.eventChan:
				switch e.EventType {
				case event_source.Update, event_source.Add:
					if _, err := r.Refresh(reconcile.Request{NamespacedName: e.Loc}, e.Info); err != nil {
						fmt.Printf("error:%v", err)
					}
				}
			}
		}
	}()
}

func (r *ReconcileSmartLimiter) Refresh(request reconcile.Request, args map[string]string) (reconcile.Result, error) {
	_, ok := r.metricInfo.Get(request.Namespace + "/" + request.Name)
	if !ok {
		r.metricInfo.Set(request.Namespace+"/"+request.Name, &model.Endpoints{
			Location: request.NamespacedName,
			Info:     args,
		})
	} else {
		if i, ok := r.metricInfo.Get(request.Namespace + "/" + request.Name); ok {
			if ep, ok := i.(*model.Endpoints); ok {
				ep.Lock.Lock()
				for key, value := range args {
					ep.Info[key] = value
				}
				ep.Lock.Unlock()
			}
		}
	}

	instance := &microservicev1alpha1.SmartLimiter{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if result, err := r.refresh(instance); err == nil {
		return result, nil
	} else {
		return reconcile.Result{}, err
	}
}

func (r *ReconcileSmartLimiter) refresh(instance *microservicev1alpha1.SmartLimiter) (reconcile.Result, error) {
	loc := types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}
	material := r.getMaterial(loc)
	if instance.Spec.Descriptors == nil {
		return reconcile.Result{}, util.Error{M: "invalid rateLimit spec"}
	}
	rateLimitConf := instance.Spec

	var ef *v1alpha3.EnvoyFilter
	var descriptor []*microservicev1alpha1.SmartLimitDescriptor

	backend := r.env.Config.Limiter.Backend
	if backend == config.Limiter_netEaseLocalFlowControl {
		ef, descriptor = r.GenerateNeteaseFlowControl(rateLimitConf, material, instance)
	}

	if backend == config.Limiter_envoyLocalRateLimit {
		ef, descriptor = r.GenerateEnvoyLocalLimit(rateLimitConf, material, instance)
	}

	if ef != nil {
		result, err := refreshEnvoyFilter(instance, r, ef)
		if err != nil {
			return result, err
		}
	}

	instance.Status = microservicev1alpha1.SmartLimiterStatus{
		RatelimitStatus: descriptor,
		EndPointStatus:  material,
	}
	if err := r.client.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func svcToName(svc *v1.Service) string {
	for _, p := range svc.Spec.Ports {
		if isHttp(p.Name) {
			return fmt.Sprintf("inbound|http|%d", p.Port)
		}
	}
	return ""
}

func isHttp(n string) bool {
	return strings.HasPrefix(n, "http")
}
