/*
* @Author: yangdihang
* @Date: 2020/11/19
 */

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	networkingapi "istio.io/api/networking/v1alpha3"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	slime_model "slime.io/slime/framework/model"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/util"
	microservicev1alpha2 "slime.io/slime/modules/limiter/api/v1alpha2"
	"slime.io/slime/modules/limiter/model"
)

// WatchMetric consume metric from chan, which is produced by producer
func (r *SmartLimiterReconciler) WatchMetric(ctx context.Context) {
	log := log.WithField("reporter", "SmartLimiterReconciler").WithField("function", "WatchMetric")
	log.Infof("start watching metric")

	for {
		select {
		case <-ctx.Done():
			log.Infof("context is closed, break process loop")
			return
		case metric, ok := <-r.watcherMetricChan:
			if !ok {
				log.Warningf("watcher mertic channel closed, break process loop")
				return
			}
			log.Debugf("get metric from watcherMetricChan, %+v", metric)
			r.ConsumeMetric(metric)
		case metric, ok := <-r.tickerMetricChan:
			if !ok {
				log.Warningf("ticker metric channel closed, break process loop")
				return
			}
			log.Debugf("get metric from tickerMetricChan, %+v", metric)
			r.ConsumeMetric(metric)
		}
	}
}

func (r *SmartLimiterReconciler) ConsumeMetric(metricMap metric.Metric) {
	r.Lock()
	defer r.Unlock()

	for metaInfo, results := range metricMap {
		Info := make(map[string]string)
		meta := &StaticMeta{}
		if err := json.Unmarshal([]byte(metaInfo), meta); err != nil {
			log.Errorf("unmarshal static meta info err, %+v", err.Error())
			continue
		}
		// exact info from meta
		namespace, name := meta.Namespace, meta.Name
		loc := types.NamespacedName{Namespace: namespace, Name: name}
		nPod, isGroup := meta.NPod, meta.IsGroup

		for _, result := range results {
			metricName := result.Name
			metricValue := ""

			if !isGroup[metricName] {
				if len(result.Value) != 1 {
					continue
				}
				for _, value := range result.Value {
					metricValue = value
				}
				Info[metricName] = metricValue
			} else {
				Info = result.Value
			}
		}

		// record the number of pods , specified by subset
		for subset, number := range nPod {
			if number == 0 {
				continue
			}
			Info[subset] = strconv.Itoa(number)
		}
		log.Debugf("exact metric info, %v", Info)
		// use info to refresh SmartLimiter's status
		if _, err := r.Refresh(reconcile.Request{NamespacedName: loc}, Info); err != nil {
			log.Errorf("refresh error:%v", err)
		}
	}
}

func (r *SmartLimiterReconciler) Refresh(request reconcile.Request, args map[string]string) (reconcile.Result, error) {
	_, ok := r.metricInfo.Get(request.Namespace + "/" + request.Name)
	if !ok {
		r.metricInfo.Set(request.Namespace+"/"+request.Name, &slime_model.Endpoints{
			Location: request.NamespacedName,
			Info:     args,
		})
	} else {
		if ep, ok := r.metricInfo.Get(request.Namespace + "/" + request.Name); ok {
			ep.Lock.Lock()
			for key, value := range args {
				ep.Info[key] = value
			}
			ep.Lock.Unlock()

		}
	}

	instance := &microservicev1alpha2.SmartLimiter{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		} else {
			// Error reading the object - requeue the request.
			return reconcile.Result{}, err
		}
	}

	// if rev not in scope, skip
	if !r.env.RevInScope(slime_model.IstioRevFromLabel(instance.Labels)) {
		log.Debugf("existing smartlimiter %v istiorev %s but our %s, skip ...",
			request.NamespacedName, slime_model.IstioRevFromLabel(instance.Labels), r.env.IstioRev())
		return ctrl.Result{}, nil
	}

	if result, err := r.refresh(instance); err == nil {
		return result, nil
	} else {
		return reconcile.Result{}, err
	}
}

// refresh envoy filters and configmap
func (r *SmartLimiterReconciler) refresh(instance *microservicev1alpha2.SmartLimiter) (reconcile.Result, error) {
	var err error
	loc := types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}
	material := r.getMaterial(loc)
	if instance.Spec.Sets == nil {
		return reconcile.Result{}, util.Error{M: "invalid rateLimit spec with none sets"}
	}
	spec := instance.Spec

	var efs map[string]*networkingapi.EnvoyFilter
	var descriptor map[string]*microservicev1alpha2.SmartLimitDescriptors
	var gdesc []*model.Descriptor

	efs, descriptor, gdesc, err = r.GenerateEnvoyConfigs(spec, material, loc)
	if err != nil {
		return reconcile.Result{}, err
	}
	for k, ef := range efs {
		var efcr *networkingv1alpha3.EnvoyFilter
		if k == util.WellknownBaseSet {
			efcr = &networkingv1alpha3.EnvoyFilter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s.%s.ratelimit", instance.Name, instance.Namespace),
					Namespace: instance.Namespace,
				},
			}
		} else {
			efcr = &networkingv1alpha3.EnvoyFilter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s.%s.%s.ratelimit", instance.Name, instance.Namespace, k),
					Namespace: instance.Namespace,
				},
			}
		}
		_, err = refreshEnvoyFilter(instance, r, efcr, ef)
		if err != nil {
			log.Errorf("generated/deleted EnvoyFilter %s failed:%+v", efcr.Name, err)
		}
	}
	if !r.cfg.GetDisableGlobalRateLimit() {
		refreshConfigMap(gdesc, r, loc)
	} else {
		log.Debugf("global rate limiter is closed")
	}
	instance.Status = microservicev1alpha2.SmartLimiterStatus{
		RatelimitStatus: descriptor,
		MetricStatus:    material,
	}
	if err = r.Client.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

// TODO different with old version
// the function will not trigger if subset is changed
// if subset is deleted, how to delete the exist envoyfilters, add anohter function to delete the efs ?
func (r *SmartLimiterReconciler) subscribe(host string, subset interface{}) {
	if name, ns, ok := util.IsK8SService(host); ok {
		loc := types.NamespacedName{Name: name, Namespace: ns}
		instance := &microservicev1alpha2.SmartLimiter{}
		err := r.Client.Get(context.TODO(), loc, instance)
		if err != nil {
			if !errors.IsNotFound(err) {
				log.Errorf("failed to get smartlimiter, host:%s, %+v", host, err)
			}
		} else {
			_, _ = r.refresh(instance)
		}
	}
}

func (r *SmartLimiterReconciler) getMaterial(loc types.NamespacedName) map[string]string {
	if ep, ok := r.metricInfo.Get(loc.Namespace + "/" + loc.Name); ok {
		return util.CopyMap(ep.Info)
	}
	return nil
}

func refreshEnvoyFilter(instance *microservicev1alpha2.SmartLimiter, r *SmartLimiterReconciler, obj *networkingv1alpha3.EnvoyFilter, ef *networkingapi.EnvoyFilter) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(instance, obj, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}
	loc := types.NamespacedName{Name: obj.Name, Namespace: instance.Namespace}
	istioRev := slime_model.IstioRevFromLabel(instance.Labels)
	slime_model.PatchIstioRevLabel(&obj.Labels, istioRev)
	slime_model.PatchObjectMeta(&obj.ObjectMeta, &instance.ObjectMeta)

	found := &networkingv1alpha3.EnvoyFilter{}
	if err := r.Client.Get(context.TODO(), loc, found); err != nil {
		if errors.IsNotFound(err) {
			found = nil
			err = nil
		} else {
			return reconcile.Result{}, fmt.Errorf("get envoyfilter err: %+v", err.Error())
		}
	}

	// envoyfilter is found or not
	if found == nil {
		// found is nil and obj's spec is not nil , create envoyFilter
		if ef != nil {
			obj.Spec = *ef
			if err := r.Client.Create(context.TODO(), obj); err != nil {
				EnvoyFilterCreationsFailed.Increment()
				return reconcile.Result{}, fmt.Errorf("creating a new EnvoyFilter err, %+v", err.Error())
			}
			EnvoyFilterCreations.Increment()
			log.Infof("creating a new EnvoyFilter,%+v", loc)
			return reconcile.Result{}, nil
		} else {
			log.Debugf("envoyfilter %+v is not found, and obj spec is nil, skip ", loc)
		}
	} else if foundRev := slime_model.IstioRevFromLabel(found.Labels); !r.env.RevInScope(foundRev) {
		log.Debugf("existing envoyfilter %v istioRev %s but our %s, skip ...",
			loc, foundRev, r.env.IstioRev())
	} else {
		foundSpec, err := json.Marshal(found.Spec)
		if err != nil {
			log.Errorf("marshal found.spec err: %+v", err)
		}
		objSpec, err := json.Marshal(ef)
		if err != nil {
			log.Errorf("marshal obj.spec err: %+v", err)
		}
		// spec is not nil , update
		if ef != nil {
			if !reflect.DeepEqual(string(foundSpec), string(objSpec)) || !reflect.DeepEqual(found.Labels, obj.Labels) {
				obj.ResourceVersion = found.ResourceVersion
				err := r.Client.Update(context.TODO(), obj)
				if err != nil {
					log.Errorf("update envoyfilter err: %+v", err.Error())
					return reconcile.Result{}, err
				}
				EnvoyfilterRefreshes.Increment()
				log.Infof("Update a new EnvoyFilter succeed,%v", loc)
				// Pod created successfully - don't requeue
				return reconcile.Result{}, nil
			}
		} else {
			log.Infof("obj sepec is nil, delete obj %s/%s", obj.Namespace, obj.Name)
			// spec is nil , delete
			err := r.Client.Delete(context.TODO(), obj)
			if errors.IsNotFound(err) {
				err = nil
			}
			EnvoyfilterDeletions.Increment()
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
