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
	"sort"
	"strings"
	"sync"
	"time"

	"slime.io/slime/framework/model"
	"slime.io/slime/framework/model/metric"

	istio "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"slime.io/slime/framework/apis/networking/v1alpha3"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/util"

	lazyloadv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
	modmodel "slime.io/slime/modules/lazyload/model"
)

// ServicefenceReconciler reconciles a Servicefence object
type ServicefenceReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	cfg                  *lazyloadv1alpha1.Fence
	env                  bootstrap.Environment
	interestMeta         map[string]bool
	interestMetaCopy     map[string]bool // for outside read
	watcherMetricChan    <-chan metric.Metric
	tickerMetricChan     <-chan metric.Metric
	reconcileLock        sync.RWMutex
	staleNamespaces      map[string]bool
	enabledNamespaces    map[string]bool
	nsSvcCache           *NsSvcCache
	labelSvcCache        *LabelSvcCache
	defaultAddNamespaces []string
	doAliasRules         []*domainAliasRule
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(cfg *lazyloadv1alpha1.Fence, mgr manager.Manager, env bootstrap.Environment) *ServicefenceReconciler {
	log := modmodel.ModuleLog.WithField(model.LogFieldKeyFunction, "NewReconciler")

	// generate producer config
	pc, err := newProducerConfig(env)
	if err != nil {
		log.Errorf("%v", err)
		return nil
	}

	r := &ServicefenceReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		env:                  env,
		interestMeta:         map[string]bool{},
		interestMetaCopy:     map[string]bool{},
		watcherMetricChan:    pc.WatcherProducerConfig.MetricChan,
		tickerMetricChan:     pc.TickerProducerConfig.MetricChan,
		staleNamespaces:      map[string]bool{},
		enabledNamespaces:    map[string]bool{},
		defaultAddNamespaces: []string{env.Config.Global.IstioNamespace, env.Config.Global.SlimeNamespace},
		doAliasRules:         newDomainAliasRules(cfg.DomainAliases),
		cfg:                  cfg,
	}

	// start service related cache
	r.nsSvcCache, r.labelSvcCache, err = newSvcCache(env.K8SClient)
	if err != nil {
		log.Errorf("init LabelSvcCache err: %v", err)
		return nil
	}

	// reconciler defines producer metric handler
	pc.WatcherProducerConfig.NeedUpdateMetricHandler = r.handleWatcherEvent
	pc.TickerProducerConfig.NeedUpdateMetricHandler = r.handleTickerEvent

	// start producer
	metric.NewProducer(pc)
	log.Infof("producers starts")

	if env.Config.Metric != nil || env.Config.Global.Misc["metricSourceType"] == MetricSourceTypeAccesslog {
		go r.WatchMetric()
	} else {
		log.Warningf("watching metric is not running")
	}

	return r
}

// +kubebuilder:rbac:groups=microservice.slime.io,resources=servicefences,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io,resources=servicefences/status,verbs=get;update;patch

func (r *ServicefenceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	log := modmodel.ModuleLog.WithField(model.LogFieldKeyResource, req.NamespacedName)

	// Fetch the ServiceFence instance
	instance := &lazyloadv1alpha1.ServiceFence{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	// TODO watch sidecar
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO should be recovered? maybe we should call refreshFenceStatusOfService here
			log.Info("serviceFence is deleted")
			// r.interestMeta.Pop(req.NamespacedName.String())
			delete(r.interestMeta, req.NamespacedName.String())
			r.updateInterestMetaCopy()
			return r.refreshFenceStatusOfService(context.TODO(), nil, req.NamespacedName)
		} else {
			log.Errorf("get serviceFence error,%+v", err)
			return reconcile.Result{}, err
		}
	}

	if rev := model.IstioRevFromLabel(instance.Labels); !r.env.RevInScope(rev) { // remove watch ?
		log.Infof("exsiting sf %v istioRev %s but our %s, skip...",
			req.NamespacedName, rev, r.env.IstioRev())
		return reconcile.Result{}, nil
	}
	log.Infof("ServicefenceReconciler got serviceFence request, %+v", req.NamespacedName)

	// 资源更新
	diff := r.updateVisitedHostStatus(instance)
	r.recordVisitor(instance, diff)
	if instance.Spec.Enable {
		err = r.refreshSidecar(instance)
		r.interestMeta[req.NamespacedName.String()] = true
		r.updateInterestMetaCopy()
	}

	return ctrl.Result{}, err
}

func (r *ServicefenceReconciler) updateInterestMetaCopy() {
	newInterestMeta := make(map[string]bool)
	for k, v := range r.interestMeta {
		newInterestMeta[k] = v
	}
	r.interestMetaCopy = newInterestMeta
}

func (r *ServicefenceReconciler) getInterestMeta() map[string]bool {
	r.reconcileLock.RLock()
	defer r.reconcileLock.RUnlock()
	return r.interestMetaCopy
}

func (r *ServicefenceReconciler) refreshSidecar(instance *lazyloadv1alpha1.ServiceFence) error {
	log := log.WithField("reporter", "ServicefenceReconciler").WithField("function", "refreshSidecar")
	sidecar, err := r.newSidecar(instance, r.env)
	if err != nil {
		log.Errorf("servicefence generate sidecar failed, %+v", err)
		return err
	}
	if sidecar == nil {
		return nil
	}
	// Set VisitedHost instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, sidecar, r.Scheme); err != nil {
		log.Errorf("attach ownerReference to sidecar failed, %+v", err)
		return err
	}
	sfRev := model.IstioRevFromLabel(instance.Labels)
	model.PatchIstioRevLabel(&sidecar.Labels, sfRev)

	// Check if this Pod already exists
	found := &v1alpha3.Sidecar{}
	nsName := types.NamespacedName{Name: sidecar.Name, Namespace: sidecar.Namespace}
	err = r.Client.Get(context.TODO(), nsName, found)

	if err != nil {
		if errors.IsNotFound(err) {
			found = nil
			err = nil
		} else {
			return err
		}
	}

	if found == nil {
		log.Infof("Creating a new Sidecar in %s:%s", sidecar.Namespace, sidecar.Name)
		err = r.Client.Create(context.TODO(), sidecar)
		if err != nil {
			return err
		}
	} else if rev := model.IstioRevFromLabel(found.Labels); rev != sfRev {
		log.Infof("existed sidecar %v istioRev %s but our rev %s, skip update ...",
			nsName, rev, sfRev)
	} else {
		if !reflect.DeepEqual(found.Spec, sidecar.Spec) {
			log.Infof("Update a Sidecar in %s:%s", sidecar.Namespace, sidecar.Name)
			sidecar.ResourceVersion = found.ResourceVersion
			err = r.Client.Update(context.TODO(), sidecar)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// recordVisitor update the dest servicefences' visitor according to src sf's visit diff
func (r *ServicefenceReconciler) recordVisitor(sf *lazyloadv1alpha1.ServiceFence, diff Diff) {
	for _, addHost := range diff.Added {
		destSf := r.prepareDestFence(sf, addHost)
		if destSf == nil {
			continue
		}
		destSf.Status.Visitor[sf.Namespace+"/"+sf.Name] = true
		_ = r.Client.Status().Update(context.TODO(), destSf)
	}

	for _, delHost := range diff.Deleted {
		destSf := r.prepareDestFence(sf, delHost)
		if destSf == nil {
			continue
		}
		delete(destSf.Status.Visitor, sf.Namespace+"/"+sf.Name)
		_ = r.Client.Status().Update(context.TODO(), destSf)
	}
}

// prepareDestFence prepares servicefence of specified host
func (r *ServicefenceReconciler) prepareDestFence(srcSf *lazyloadv1alpha1.ServiceFence, h string) *lazyloadv1alpha1.ServiceFence {
	nsName := parseHost(srcSf.Namespace, h)
	if nsName == nil {
		return nil
	}

	svc := &corev1.Service{}
	if err := r.Client.Get(context.TODO(), *nsName, svc); err != nil {
		// XXX err handle
		return nil
	}

	destSf := &lazyloadv1alpha1.ServiceFence{}
retry: // FIXME fix infinite loop
	err := r.Client.Get(context.TODO(), *nsName, destSf)
	if err != nil {
		if errors.IsNotFound(err) {
			// XXX maybe should not auto create
			destSf.Name = nsName.Name
			destSf.Namespace = nsName.Namespace
			// XXX set controlled by
			// XXX patch rev
			if err = r.Client.Create(context.TODO(), destSf); err != nil {
				goto retry
			}
		} else {
			return nil
		}
	}

	if destSf.Status.Visitor == nil {
		destSf.Status.Visitor = make(map[string]bool)
	}
	return destSf
}

func parseHost(sourceNs, h string) *types.NamespacedName {
	s := strings.Split(h, ".")
	if len(s) == 5 || len(s) == 2 { // shortname.ns or full(shortname.ns.svc.cluster.local)
		return &types.NamespacedName{
			Namespace: s[1],
			Name:      s[0],
		}
	}
	if len(s) == 1 { // shortname
		return &types.NamespacedName{
			Namespace: sourceNs,
			Name:      s[0],
		}
	}
	return nil // unknown host format, maybe external host
}

func (r *ServicefenceReconciler) updateVisitedHostStatus(sf *lazyloadv1alpha1.ServiceFence) Diff {
	domains := r.genDomains(sf, r.doAliasRules)

	delta := Diff{
		Deleted: make([]string, 0),
		Added:   make([]string, 0),
	}
	for k, dest := range sf.Status.Domains {
		if _, ok := domains[k]; !ok {
			if dest.Status == lazyloadv1alpha1.Destinations_ACTIVE {
				// active -> pending
				domains[k] = &lazyloadv1alpha1.Destinations{
					Hosts:  dest.Hosts,
					Status: lazyloadv1alpha1.Destinations_EXPIREWAIT,
				}
			} else {
				// pending -> delete
				delta.Deleted = append(delta.Deleted, k)
			}
		}
	}
	for k := range domains {
		if _, ok := sf.Status.Domains[k]; !ok {
			delta.Added = append(delta.Added, k)
		}
	}
	sf.Status.Domains = domains

	_ = r.Client.Status().Update(context.TODO(), sf)

	return delta
}

func (r *ServicefenceReconciler) genDomains(sf *lazyloadv1alpha1.ServiceFence, rules []*domainAliasRule) map[string]*lazyloadv1alpha1.Destinations {
	domains := make(map[string]*lazyloadv1alpha1.Destinations)

	addDomainsWithHost(domains, sf, r.nsSvcCache, rules)
	addDomainsWithLabelSelector(domains, sf, r.labelSvcCache, rules)
	addDomainsWithMetricStatus(domains, sf, rules)

	return domains
}

// update domains with spec.host
func addDomainsWithHost(domains map[string]*lazyloadv1alpha1.Destinations, sf *lazyloadv1alpha1.ServiceFence, nsSvcCache *NsSvcCache,
	rules []*domainAliasRule,
) {
	checkStatus := func(now int64, strategy *lazyloadv1alpha1.RecyclingStrategy) lazyloadv1alpha1.Destinations_Status {
		switch {
		case strategy.Stable != nil:
			// ...
		case strategy.Deadline != nil:
			if now > strategy.Deadline.Expire.Seconds {
				return lazyloadv1alpha1.Destinations_EXPIRE
			}
		case strategy.Auto != nil:
			if strategy.RecentlyCalled != nil {
				if now-strategy.RecentlyCalled.Seconds > strategy.Auto.Duration.Seconds {
					return lazyloadv1alpha1.Destinations_EXPIRE
				}
			}
		}
		return lazyloadv1alpha1.Destinations_ACTIVE
	}

	for h, strategy := range sf.Spec.Host {
		if strings.HasSuffix(h, "/*") {
			// handle namespace level host, like 'default/*'
			handleNsHost(h, domains, nsSvcCache, rules)
		} else {
			// handle service level host, like 'a.default.svc.cluster.local' or 'www.netease.com'
			handleSvcHost(h, strategy, checkStatus, domains, sf, rules)
		}
	}
}

func handleNsHost(h string, domains map[string]*lazyloadv1alpha1.Destinations, nsSvcCache *NsSvcCache, rules []*domainAliasRule) {
	hostParts := strings.Split(h, "/")
	if len(hostParts) != 2 {
		log.Errorf("%s is invalid host, skip", h)
		return
	}

	nsSvcCache.RLock()
	defer nsSvcCache.RUnlock()

	svcs := nsSvcCache.Data[hostParts[0]]
	var allHost []string
	for svc := range svcs {
		svcParts := strings.Split(svc, "/")
		fullHost := fmt.Sprintf("%s.%s.svc.cluster.local", svcParts[1], svcParts[0])
		if !isValidHost(fullHost) {
			continue
		}

		fullHosts := domainAddAlias(fullHost, rules)
		for _, fh := range fullHosts {
			if domains[fh] != nil {
				continue
			}
			// service relates to other services
			if hs := getDestination(fh); len(hs) > 0 {
				for i := 0; i < len(hs); {
					hParts := strings.Split(hs[i], ".")
					// ignore destSvc that in the same namespace
					if hParts[1] == hostParts[0] {
						hs[i], hs[len(hs)-1] = hs[len(hs)-1], hs[i]
						hs = hs[:len(hs)-1]
					} else {
						i++
					}
				}

				allHost = append(allHost, hs...)
			}
		}
	}
	domains[h] = &lazyloadv1alpha1.Destinations{
		Hosts:  allHost,
		Status: lazyloadv1alpha1.Destinations_ACTIVE,
	}
}

func handleSvcHost(fullHost string, strategy *lazyloadv1alpha1.RecyclingStrategy,
	checkStatus func(now int64, strategy *lazyloadv1alpha1.RecyclingStrategy) lazyloadv1alpha1.Destinations_Status,
	domains map[string]*lazyloadv1alpha1.Destinations, sf *lazyloadv1alpha1.ServiceFence, rules []*domainAliasRule,
) {
	now := time.Now().Unix()

	if !isValidHost(fullHost) {
		return
	}

	fullHosts := domainAddAlias(fullHost, rules)
	for _, fh := range fullHosts {
		if domains[fh] != nil {
			return
		}

		allHost := []string{fh}
		if hs := getDestination(fh); len(hs) > 0 {
			allHost = append(allHost, hs...)
		}

		domains[fh] = &lazyloadv1alpha1.Destinations{
			Hosts:  allHost,
			Status: checkStatus(now, strategy),
		}
	}
}

// update domains with spec.labelSelector
func addDomainsWithLabelSelector(domains map[string]*lazyloadv1alpha1.Destinations, sf *lazyloadv1alpha1.ServiceFence,
	labelSvcCache *LabelSvcCache, rules []*domainAliasRule,
) {
	labelSvcCache.RLock()
	defer labelSvcCache.RUnlock()

	for _, selector := range sf.Spec.LabelSelector {

		var result map[string]struct{}
		// generate result for this selector
		for k, v := range selector.Selector {
			label := LabelItem{
				Name:  k,
				Value: v,
			}
			svcs := labelSvcCache.Data[label]
			if svcs == nil {
				result = nil
				break
			}
			// init result
			if result == nil {
				result = make(map[string]struct{}, len(svcs))
				for svc := range svcs {
					result[svc] = struct{}{}
				}
			} else {
				// check result for other labels
				for re := range result {
					if _, ok := svcs[re]; !ok {
						// not exist svc in this label cache
						delete(result, re)
					}
				}
			}
		}

		// get hosts of each service
		for re := range result {
			subdomains := strings.Split(re, "/")
			fullHost := fmt.Sprintf("%s.%s.svc.cluster.local", subdomains[1], subdomains[0])
			if !isValidHost(fullHost) {
				continue
			}

			fullHosts := domainAddAlias(fullHost, rules)
			for _, fh := range fullHosts {
				addToDomains(domains, fh)
			}
		}

	}
}

// update domains with Status.MetricStatus
func addDomainsWithMetricStatus(domains map[string]*lazyloadv1alpha1.Destinations, sf *lazyloadv1alpha1.ServiceFence, rules []*domainAliasRule) {
	for metricName := range sf.Status.MetricStatus {
		metricName = strings.Trim(metricName, "{}")
		if !strings.HasPrefix(metricName, "destination_service") && !strings.HasPrefix(metricName, "request_host") {
			continue
		}
		// destination_service format like: "grafana.istio-system.svc.cluster.local"

		var fullHost string
		// trim ""
		if ss := strings.Split(metricName, "\""); len(ss) != 3 {
			continue
		} else {
			// remove port
			fullHost = strings.SplitN(ss[1], ":", 2)[0]
		}

		if !isValidHost(fullHost) {
			continue
		}

		fullHosts := domainAddAlias(fullHost, rules)
		for _, fh := range fullHosts {
			addToDomains(domains, fh)
		}
	}
}

func (r *ServicefenceReconciler) newSidecar(sf *lazyloadv1alpha1.ServiceFence, env bootstrap.Environment) (*v1alpha3.Sidecar, error) {
	hosts := make([]string, 0)

	if !sf.Spec.Enable {
		return nil, nil
	}

	for _, ns := range r.defaultAddNamespaces {
		hosts = append(hosts, ns+"/*")
	}

	for k, v := range sf.Status.Domains {
		if v.Status == lazyloadv1alpha1.Destinations_ACTIVE || v.Status == lazyloadv1alpha1.Destinations_EXPIREWAIT {
			if strings.HasSuffix(k, "/*") {
				if !r.isDefaultAddNs(k) {
					hosts = append(hosts, k)
				}
			}

			for _, h := range v.Hosts {
				hosts = append(hosts, "*/"+h)
			}
		}
	}

	// check whether using namespace global-sidecar
	// if so, init config of sidecar will adds */global-sidecar.${svf.ns}.svc.cluster.local
	if env.Config.Global.Misc["globalSidecarMode"] == "namespace" {
		hosts = append(hosts, fmt.Sprintf("*/global-sidecar.%s.svc.cluster.local", sf.Namespace))
	}

	// remove duplicated hosts
	noDupHosts := make([]string, 0, len(hosts))
	temp := map[string]struct{}{}
	for _, item := range hosts {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			noDupHosts = append(noDupHosts, item)
		}
	}
	hosts = noDupHosts

	// sort hosts so that it follows the Equals semantics
	sort.Strings(hosts)

	sidecar := &istio.Sidecar{
		WorkloadSelector: &istio.WorkloadSelector{
			Labels: map[string]string{},
		},
		Egress: []*istio.IstioEgressListener{
			{
				// Bind:  "0.0.0.0",
				Hosts: hosts,
			},
		},
	}

	// Fetch the Service instance
	nsName := types.NamespacedName{
		Name:      sf.Name,
		Namespace: sf.Namespace,
	}

	svc := &corev1.Service{}
	if err := r.Client.Get(context.TODO(), nsName, svc); err != nil {
		if errors.IsNotFound(err) {
			log.Warningf("cannot find service %s for servicefence, skip sidecar generating", nsName)
			return nil, nil
		} else {
			log.Errorf("get service %s error, %+v", nsName, err)
			return nil, err
		}
	}

	// generate sidecar.spec.workloadSelector
	// priority: sf.spec.workloadSelector.labels > sf.spec.workloadSelector.fromService
	if sf.Spec.WorkloadSelector != nil && len(sf.Spec.WorkloadSelector.Labels) > 0 {
		// sidecar.WorkloadSelector.Labels = sf.Spec.WorkloadSelector.Labels
		for k, v := range sf.Spec.WorkloadSelector.Labels {
			sidecar.WorkloadSelector.Labels[k] = v
		}
	} else if sf.Spec.WorkloadSelector != nil && sf.Spec.WorkloadSelector.FromService {
		// sidecar.WorkloadSelector.Labels = svc.Spec.Selector
		for k, v := range svc.Spec.Selector {
			sidecar.WorkloadSelector.Labels[k] = v
		}
	} else {
		// compatible with old version lazyload
		sidecar.WorkloadSelector.Labels[env.Config.Global.Service] = svc.Name
	}

	spec, err := util.ProtoToMap(sidecar)
	if err != nil {
		return nil, err
	}
	ret := &v1alpha3.Sidecar{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sf.Name,
			Namespace: sf.Namespace,
		},
		Spec: spec,
	}
	return ret, nil
}

func (r *ServicefenceReconciler) Subscribe(host string, destination interface{}) { // FIXME not used?
	svc, namespace, ok := util.IsK8SService(host)
	if !ok {
		return
	}

	sf := &lazyloadv1alpha1.ServiceFence{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: svc, Namespace: namespace}, sf); err != nil {
		return
	}
	istioRev := model.IstioRevFromLabel(sf.Labels)
	if !r.env.RevInScope(istioRev) {
		return
	}

	for k := range sf.Status.Visitor {
		i := strings.Index(k, "/")
		if i < 0 {
			continue
		}

		visitorSf := &lazyloadv1alpha1.ServiceFence{}
		visitorNamespace := k[:i]
		visitorName := k[i+1:]

		err := r.Client.Get(context.TODO(), types.NamespacedName{Name: visitorName, Namespace: visitorNamespace}, visitorSf)
		if err != nil {
			return
		}
		visitorRev := model.IstioRevFromLabel(visitorSf.Labels)
		if !r.env.RevInScope(visitorRev) {
			return
		}

		r.updateVisitedHostStatus(visitorSf)

		sidecar, err := r.newSidecar(visitorSf, r.env)
		if sidecar == nil {
			continue
		}
		if err != nil {
			return
		}

		// Set VisitedHost instance as the owner and controller
		if err := controllerutil.SetControllerReference(visitorSf, sidecar, r.Scheme); err != nil {
			return
		}
		model.PatchIstioRevLabel(&sidecar.Labels, visitorRev)

		// Check for existence
		found := &v1alpha3.Sidecar{}
		nsName := types.NamespacedName{Name: sidecar.Name, Namespace: sidecar.Namespace}
		err = r.Client.Get(context.TODO(), nsName, found)
		if err != nil {
			if errors.IsNotFound(err) {
				found = nil
				err = nil
			} else {
				return
			}
		}

		if found == nil {
			err = r.Client.Create(context.TODO(), sidecar)
			if err != nil {
				log.Errorf("create sidecar %+v met err %v", sidecar, err)
			}
		} else if scRev := model.IstioRevFromLabel(found.Labels); scRev != visitorRev {
			log.Infof("existing sidecar %v istioRev %s but our %s, skip ...",
				nsName, scRev, visitorRev)
		} else {
			if !reflect.DeepEqual(found.Spec, sidecar.Spec) {
				sidecar.ResourceVersion = found.ResourceVersion
				err = r.Client.Update(context.TODO(), sidecar)

				if err != nil {
					log.Errorf("create sidecar %+v met err %v", sidecar, err)
				}
			}
		}
		return
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

func (r *ServicefenceReconciler) isDefaultAddNs(ns string) bool {
	for _, defaultNs := range r.defaultAddNamespaces {
		if defaultNs == ns {
			return true
		}
	}
	return false
}

func (r *ServicefenceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lazyloadv1alpha1.ServiceFence{}).
		Complete(r)
}
