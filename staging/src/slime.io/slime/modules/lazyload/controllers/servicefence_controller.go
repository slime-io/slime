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

	networkingapi "istio.io/api/networking/v1alpha3"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/modules/lazyload/api/config"
	lazyloadv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
	modmodel "slime.io/slime/modules/lazyload/model"
)

// ServicefenceReconciler reconciles a Servicefence object
type ServicefenceReconciler struct {
	client.Client

	Scheme                     *runtime.Scheme
	cfg                        *config.Fence
	env                        bootstrap.Environment
	interestMeta               map[string]bool
	interestMetaCopy           map[string]bool // for outside read
	watcherMetricChan          <-chan metric.Metric
	tickerMetricChan           <-chan metric.Metric
	reconcileLock              sync.RWMutex
	nsSvcCache                 *NsSvcCache
	labelSvcCache              *LabelSvcCache
	portProtocolCache          *PortProtocolCache
	defaultAddNamespaces       []string
	doAliasRules               []*domainAliasRule
	ipTofence                  *IpTofence
	fenceToIp                  *FenceToIp
	workloadFenceLabelKey      string
	workloadFenceLabelKeyAlias string
	ipToSvcCache               *IpToSvcCache
	svcToIpsCache              *SvcToIpsCache
	factory                    informers.SharedInformerFactory
}

type ReconcilerOpts func(*ServicefenceReconciler)

func ReconcilerWithEnv(env bootstrap.Environment) ReconcilerOpts {
	return func(sr *ServicefenceReconciler) {
		sr.env = env
		sr.defaultAddNamespaces = append(sr.defaultAddNamespaces, env.Config.Global.IstioNamespace)
		if env.Config.Global.IstioNamespace != env.Config.Global.SlimeNamespace {
			sr.defaultAddNamespaces = append(sr.defaultAddNamespaces, env.Config.Global.SlimeNamespace)
		}
	}
}

func ReconcilerWithCfg(cfg *config.Fence) ReconcilerOpts {
	return func(sr *ServicefenceReconciler) {
		sr.cfg = cfg
		sr.doAliasRules = newDomainAliasRules(cfg.DomainAliases)
	}
}

func ReconcilerWithProducerConfig(pc *metric.ProducerConfig) ReconcilerOpts {
	return func(sr *ServicefenceReconciler) {
		sr.watcherMetricChan = pc.WatcherProducerConfig.MetricChan
		sr.tickerMetricChan = pc.TickerProducerConfig.MetricChan
		// reconciler defines producer metric handler
		pc.WatcherProducerConfig.NeedUpdateMetricHandler = sr.handleWatcherEvent
		pc.TickerProducerConfig.NeedUpdateMetricHandler = sr.handleTickerEvent

		// reconciler defines log handler
		for i := range pc.AccessLogSourceConfig.AccessLogConvertorConfigs {
			pc.AccessLogSourceConfig.AccessLogConvertorConfigs[i].Handler = sr.LogHandler
		}
	}
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(opts ...ReconcilerOpts) *ServicefenceReconciler {
	r := &ServicefenceReconciler{
		interestMeta:      map[string]bool{},
		interestMetaCopy:  map[string]bool{},
		nsSvcCache:        &NsSvcCache{Data: map[string]map[string]struct{}{}},
		labelSvcCache:     &LabelSvcCache{Data: map[LabelItem]map[string]struct{}{}},
		portProtocolCache: &PortProtocolCache{Data: map[int32]map[Protocol]int32{}},
		ipTofence:         &IpTofence{Data: map[string]types.NamespacedName{}},
		fenceToIp:         &FenceToIp{Data: map[types.NamespacedName]map[string]struct{}{}},
		ipToSvcCache:      &IpToSvcCache{Data: map[string]map[string]struct{}{}},
		svcToIpsCache:     &SvcToIpsCache{Data: map[string][]string{}},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Clear do anything since releading is not supported by framework
func (r *ServicefenceReconciler) Clear() {
	// r.reconcileLock.Lock()
	// defer r.reconcileLock.Unlock()
	//
	// reset cache
	// r.interestMeta = map[string]bool{}
	// r.interestMetaCopy = map[string]bool{}
	// r.staleNamespaces = map[string]bool{}
	// r.enabledNamespaces = map[string]bool{}
}

// +kubebuilder:rbac:groups=microservice.slime.io,resources=servicefences,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microservice.slime.io,resources=servicefences/status,verbs=get;update;patch

func (r *ServicefenceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := modmodel.ModuleLog.WithField(model.LogFieldKeyResource, req.NamespacedName)

	log.Debugf("reconcile svf %s", req)
	// Fetch the ServiceFence instance
	instance := &lazyloadv1alpha1.ServiceFence{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)

	r.reconcileLock.Lock()
	defer r.reconcileLock.Unlock()

	if err != nil {
		if errors.IsNotFound(err) {
			// we should delete info in interestMeta if svf is deleted
			log.Infof("serviceFence %+v is deleted", req.NamespacedName)
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
	log.Infof("serviceFence %+v is added or update", req.NamespacedName)

	// 资源更新
	r.updateServicefenceDomain(instance)

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
	found := &networkingv1alpha3.Sidecar{}
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
			SidecarFailedCreations.Increment()
			return err
		}
		SidecarCreations.Increment()
	} else if foundRev := model.IstioRevFromLabel(found.Labels); !r.env.RevInScope(foundRev) {
		log.Infof("existed sidecar %v istioRev %s but our rev %s, skip update ...",
			nsName, foundRev, r.env.IstioRev())
	} else {
		if !reflect.DeepEqual(found.Spec, sidecar.Spec) || !reflect.DeepEqual(found.Labels, sidecar.Labels) {
			log.Infof("Update a Sidecar in %s:%s", sidecar.Namespace, sidecar.Name)
			sidecar.ResourceVersion = found.ResourceVersion
			err = r.Client.Update(context.TODO(), sidecar)
			if err != nil {
				return err
			}
			SidecarRefreshes.Increment()
		}
	}
	return nil
}

func (r *ServicefenceReconciler) updateServicefenceDomain(sf *lazyloadv1alpha1.ServiceFence) {
	domains := r.genDomains(sf, r.doAliasRules)

	for k, dest := range sf.Status.Domains {
		if _, ok := domains[k]; !ok {
			if dest.Status == lazyloadv1alpha1.Destinations_ACTIVE {
				// active -> pending
				domains[k] = &lazyloadv1alpha1.Destinations{
					Hosts:  dest.Hosts,
					Status: lazyloadv1alpha1.Destinations_EXPIREWAIT,
				}
			}
		}
	}
	sf.Status.Domains = domains

	_ = r.Client.Status().Update(context.TODO(), sf)
	ServiceFenceRefresh.Increment()
	return
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

func (r *ServicefenceReconciler) newSidecar(sf *lazyloadv1alpha1.ServiceFence, env bootstrap.Environment) (*networkingv1alpha3.Sidecar, error) {
	hosts := make([]string, 0)

	if !sf.Spec.Enable {
		log.Debugf("svf %s/%s not enable", sf.Namespace, sf.Name)
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
	var globalSidecarNs string

	if r.cfg.GlobalSidecarMode == "namespace" {
		globalSidecarNs = sf.Namespace
	} else if clusterGsNamespace := r.cfg.GetClusterGsNamespace(); clusterGsNamespace != env.Config.Global.SlimeNamespace {
		// all service in slime ns have been added
		globalSidecarNs = clusterGsNamespace
	}
	if globalSidecarNs != "" {
		hosts = append(hosts, fmt.Sprintf("*/global-sidecar.%s.svc.cluster.local", globalSidecarNs))
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
	log.Debugf("sort host is %+v in %s:%s", hosts, sf.Namespace, sf.Name)
	sidecar := &networkingapi.Sidecar{
		WorkloadSelector: &networkingapi.WorkloadSelector{
			Labels: map[string]string{},
		},
		Egress: []*networkingapi.IstioEgressListener{
			{
				// Bind:  "0.0.0.0",
				Hosts: hosts,
			},
		},
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
		for k, v := range svc.Spec.Selector {
			sidecar.WorkloadSelector.Labels[k] = v
		}
	} else {
		// compatible with old version lazyload
		sidecar.WorkloadSelector.Labels[env.Config.Global.Service] = sf.Name
	}

	ret := &networkingv1alpha3.Sidecar{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sf.Name,
			Namespace: sf.Namespace,
		},
		Spec: *sidecar,
	}
	return ret, nil
}

func getDestination(k string) []string {
	return controllers.HostDestinationMapping.Get(k)
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
