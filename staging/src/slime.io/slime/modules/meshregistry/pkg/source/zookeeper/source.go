/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package zookeeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/hashicorp/go-multierror"
	cmap "github.com/orcaman/concurrent-map/v2"
	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	frameworkmodel "slime.io/slime/framework/model"
	frameworkutil "slime.io/slime/framework/util"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

const (
	SourceName = "zookeeper"

	ZkPath                    = "/zk"
	ZkSimplePath              = "/zks"
	DubboCallModelPath        = "/dubboCallModel"
	SidecarDubboCallModelPath = "/sidecarDubboCallModel"

	ConsumerNode        = "consumers"
	ProviderNode        = "providers"
	ConfiguratorNode    = "configurators"
	providerPathSuffix  = "/" + ProviderNode
	consumerPathSuffix  = "/" + ConsumerNode
	disableConsumerPath = "-"
	configuratorSuffix  = "/" + ConfiguratorNode

	AttachmentDubboCallModel = "ATTACHMENT_DUBBO_CALL_MODEL"

	defaultServiceFilter = ""
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "zk")

type zkConn struct {
	conn atomic.Value // store *zk.Conn
}

func (z *zkConn) Store(conn *zk.Conn) {
	z.conn.Store(conn)
}

func (z *zkConn) Load() interface{} {
	return z.conn.Load()
}

func (z *zkConn) Children(path string) ([]string, error) {
	children, _, err := z.conn.Load().(*zk.Conn).Children(path)
	monitoring.RecordSourceClientRequest(SourceName, err == nil)
	return children, err
}

func (z *zkConn) ChildrenW(path string) ([]string, <-chan zk.Event, error) {
	children, _, c, err := z.conn.Load().(*zk.Conn).ChildrenW(path)
	monitoring.RecordSourceClientRequest(SourceName, err == nil)
	return children, c, err
}

func init() {
	source.RegisterSourceInitlizer(SourceName, source.RegistrySourceInitlizer(New))
}

type Source struct {
	args *bootstrap.ZookeeperSourceArgs

	ignoreLabelsMap map[string]string
	watchingRoot    bool // TODO useless?

	serviceMethods map[string]string

	registryServiceCache cmap.ConcurrentMap[string, cmap.ConcurrentMap[string, []dubboInstance]]
	cache                cmap.ConcurrentMap[string, cmap.ConcurrentMap[string, *ServiceEntryWithMeta]]

	sidecarCache         map[resource.FullName]SidecarWithMeta
	dubboCallModels      map[string]DubboCallModel // can only be replaced rather than being modified
	seDubboCallModels    map[resource.FullName]map[string]DubboCallModel
	changedApps          map[string]struct{}
	appSidecarUpdateTime map[string]time.Time
	dubboPortsCache      map[uint32]*networkingapi.Port

	handlers       []event.Handler
	initedCallback func(string)
	mut            sync.RWMutex

	seInitCh               chan struct{}
	initWg                 sync.WaitGroup
	refreshSidecarNotifyCh chan struct{}
	stop                   chan struct{}

	Con               *zkConn
	seMergePortMocker *source.ServiceEntryMergePortMocker

	// instanceFilter fitler which instance of a service should be include
	// Updates are only allowed when the configuration is loaded or reloaded.
	instanceFilter func(*dubboInstance) bool

	// methodLBChecker check whether the method-lb feature is enabled for the service
	// NOTICE: support dynamic config and thus should be accessed with lock.
	methodLBChecker func(*convertedServiceEntry) bool

	forceUpdateTrigger *atomic.Value // store chan struct{}
}

func New(
	moduleArgs *bootstrap.RegistryArgs,
	readyCallback func(string),
	addOnReArgs func(onReArgsCallback func(args *bootstrap.RegistryArgs)),
) (event.Source, map[string]http.HandlerFunc, bool, bool, error) {
	args := moduleArgs.ZookeeperSource
	if !args.Enabled {
		return nil, nil, false, true, nil
	}

	// XXX refactor to config
	if args.GatewayModel {
		args.SvcPort = 80
		args.InstancePortAsSvcPort = false
		args.HostSuffix = ".dubbo"
		// not fetch consumer data by default for gw
	} else {
		if args.ConsumerPath == "" {
			args.ConsumerPath = consumerPathSuffix
		}
	}
	if args.ConsumerPath == disableConsumerPath {
		args.ConsumerPath = ""
	}

	ignoreLabels := make(map[string]string, 0)
	for _, v := range args.IgnoreLabel {
		ignoreLabels[v] = v
	}

	var svcMocker *source.ServiceEntryMergePortMocker
	if args.MockServiceEntryName != "" {
		if args.MockServiceName == "" {
			return nil, nil, false, false, fmt.Errorf("args MockServiceName empty but MockServiceEntryName %s", args.MockServiceEntryName)
		}
		svcMocker = source.NewServiceEntryMergePortMocker(
			args.MockServiceEntryName, args.ResourceNs, args.MockServiceName,
			args.MockServiceMergeInstancePort, args.MockServiceMergeServicePort,
			map[string]string{
				"path":     args.MockServiceName,
				"registry": SourceName,
			})
	}

	src := &Source{
		args:                   args,
		ignoreLabelsMap:        ignoreLabels,
		initedCallback:         readyCallback,
		serviceMethods:         map[string]string{},
		registryServiceCache:   cmap.New[cmap.ConcurrentMap[string, []dubboInstance]](),
		cache:                  cmap.New[cmap.ConcurrentMap[string, *ServiceEntryWithMeta]](),
		seDubboCallModels:      map[resource.FullName]map[string]DubboCallModel{},
		appSidecarUpdateTime:   map[string]time.Time{},
		dubboPortsCache:        map[uint32]*networkingapi.Port{},
		seInitCh:               make(chan struct{}),
		stop:                   make(chan struct{}),
		watchingRoot:           false,
		refreshSidecarNotifyCh: make(chan struct{}, 1),
		Con:                    &zkConn{},
		seMergePortMocker:      svcMocker,
		forceUpdateTrigger:     &atomic.Value{},
	}
	src.forceUpdateTrigger.Store(make(chan struct{}))

	src.handlers = append(
		src.handlers,
		event.HandlerFromFn(src.serviceEntryHandlerRefreshSidecar),
	)

	src.initWg.Add(1) // ServiceEntry init-sync ready
	if args.EnableDubboSidecar {
		src.initWg.Add(1) // Sidecar init-sync ready
	}
	if src.seMergePortMocker != nil {
		src.handlers = append(src.handlers, src.seMergePortMocker)
		svcMocker.SetDispatcher(src.dispatchMergePortsServiceEntry)
		src.initWg.Add(1) // merge ports se init-sync ready
	}

	src.instanceFilter = generateInstanceFilter(args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll, args.AlwaysUseSourceScopedEpSelectors)
	src.methodLBChecker = generateMethodLBChecker(args.MethodLBServiceSelectors)

	if addOnReArgs != nil {
		addOnReArgs(func(reArgs *bootstrap.RegistryArgs) {
			src.onConfig(reArgs.ZookeeperSource)
		})
	}

	debugHandler := map[string]http.HandlerFunc{
		ZkPath:                    src.cacheJson,
		ZkSimplePath:              src.simpleCacheJson,
		DubboCallModelPath:        src.HandleDubboCallModel,
		SidecarDubboCallModelPath: src.HandleSidecarDubboCallModel,
	}

	return src, debugHandler, args.LabelPatch, false, nil
}

func (s *Source) dispatchMergePortsServiceEntry(meta resource.Metadata, se *networkingapi.ServiceEntry) {
	prepared, _ := prepareServiceEntryWithMeta(se, meta)
	ev, err := buildServiceEntryEvent(event.Updated, prepared.ServiceEntry, prepared.Meta, false)
	if err != nil {
		log.Errorf("buildSeEvent met err %v", err)
		return
	}

	for _, h := range s.handlers {
		h.Handle(ev)
	}
}

type zkLogger struct {
	conn func() string
}

func (l zkLogger) Printf(format string, args ...interface{}) {
	log.WithField("lib", "go-zk").WithField("conn", l.conn()).Infof(format, args...)
}

func (s *Source) reConFunc(reconCh chan<- struct{}) {
	monitoring.RecordSourceConnectionStatus(SourceName, false)
	if s.watchingRoot {
		return // ??
	}

	// TODO: use the zk.Conn.hostProvider.Len() replace the len(s.args.Address)?
	connectTimeout := time.Duration(len(s.args.Address)+1) * time.Second
	var curConn *zk.Conn
	if v := s.Con.Load(); v != nil {
		curConn = v.(*zk.Conn)
	}
	if curConn != nil {
		if curConn.State() == zk.StateHasSession {
			log.Infof("the state of zk conn %p is ok with sessionID: %d", curConn, curConn.SessionID())
			monitoring.RecordSourceConnectionStatus(SourceName, true)
			return
		}
		curConn.Close()
		log.Infof("close connection for zk conn %p", curConn)
	}

	for {
		var con *zk.Conn
		var err error
		var once sync.Once
		logCon := func() string { return fmt.Sprintf("%p", con) }
		con, _, err = zk.Connect(s.args.Address, time.Duration(s.args.ConnectionTimeout),
			zk.WithLogger(zkLogger{conn: logCon}),
			zk.WithNoRetryHosts(), // https://github.com/slime-io/go-zk/pull/1
			zk.WithEventCallback(func(ev zk.Event) {
				if ev.Type != zk.EventDisconnected {
					return
				}

				// wait for zk reconnected by self or create a new connection asynchronizely
				go func() {
					// use the reconnection mechanism of the current client first
					time.Sleep(connectTimeout)
					switch con.State() {
					case zk.StateHasSession:
						log.Infof("zk conn %s reconnect by self with sessionID: %d", logCon(), con.SessionID())
					default:
						// ensure that each zk connection triggers only one reconnect
						once.Do(func() {
							select {
							case reconCh <- struct{}{}:
							default:
							}
							log.Warnf("zk conn %s disconnected, already notify slime to reconnect", logCon())
						})
					}
				}()
			}))
		if err != nil {
			log.Infof("connect zk error %v", err)
			time.Sleep(time.Second)
		} else {
			// TODO: this should be done in go-zk
			connected := false
			timeout := time.After(connectTimeout)
		check:
			for {
				time.Sleep(1500 * time.Millisecond) // Wait for connecting. When go-zk connects to zk, the timeout is one second.
				connState := con.State()
				if connState == zk.StateHasSession {
					connected = true
					break
				}
				select {
				case <-timeout:
					log.Infof("zk conn %s connect timeout", logCon())
					break check
				default:
				}
			}
			if connected {
				// replace the connection
				s.Con.Store(con)
				log.Infof("zk conn %s connect to zk successfully with sessionID: %d", logCon(), con.SessionID())
				monitoring.RecordSourceConnectionStatus(SourceName, true)
				break
			}
		}
	}
}

func (s *Source) Dispatch(handler event.Handler) {
	s.handlers = append(s.handlers, handler)
}

func (s *Source) simpleCacheJson(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	iface := values.Get("iface")
	all := values.Get("all")
	var result interface{}
	if iface == "" && all == "" {
		result = s.cacheSummary()
	} else {
		result = s.cacheInfo(iface)
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal zk se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func internalCacheJson[V any](s *Source, w http.ResponseWriter, req *http.Request, cache cmap.ConcurrentMap[string, cmap.ConcurrentMap[string, V]]) {
	temp := cache
	cacheData := map[string]map[string]interface{}{}
	var result interface{}

	interfaceName := req.URL.Query().Get("interfaceName")
	if interfaceName != "" {
		newTemp := cmap.New[cmap.ConcurrentMap[string, V]]()
		if value, exist := temp.Get(interfaceName); exist {
			newTemp.Set(interfaceName, value)
		}
		temp = newTemp
	}

	temp.IterCb(func(dubboInterface string, inner cmap.ConcurrentMap[string, V]) {
		interfaceCacheData := cacheData[dubboInterface]
		if interfaceCacheData == nil {
			interfaceCacheData = map[string]interface{}{}
			cacheData[dubboInterface] = interfaceCacheData
		}
		inner.IterCb(func(host string, v V) {
			switch val := any(v).(type) {
			case *ServiceEntryWithMeta:
				sem := val
				s.mut.RLock()
				methods, ok := s.serviceMethods[host]
				s.mut.RUnlock()
				if ok && sem.Meta.Labels[dubboParamMethods] != methods {
					semCopy := *sem
					labelCopy := make(map[string]string, len(sem.Meta.Labels))
					for k, v := range sem.Meta.Labels {
						labelCopy[k] = v
					}
					labelCopy[dubboParamMethods] = methods
					semCopy.Meta.Labels = labelCopy
					sem = &semCopy
				}
				interfaceCacheData[host] = sem
			case []dubboInstance:
				services := val
				interfaceCacheData[host] = services
			}
		})
	})
	if interfaceName != "" {
		result = cacheData[interfaceName]
	} else {
		result = cacheData
	}

	b, err := json.MarshalIndent(map[string]interface{}{"cache": result}, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal zk se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func (s *Source) cacheJson(w http.ResponseWriter, req *http.Request) {
	registrySvc := req.URL.Query().Get("registry_services")
	if ok, _ := strconv.ParseBool(registrySvc); ok {
		internalCacheJson(s, w, req, s.registryServiceCache)
	} else {
		internalCacheJson(s, w, req, s.cache)
	}
}

func (s *Source) isPollingMode() bool {
	return s.args.Mode == source.ModePolling
}

func (s *Source) Start() {
	if s.initedCallback != nil {
		t0 := time.Now()
		go func() {
			s.initWg.Wait()
			monitoring.RecordReady(SourceName, t0, time.Now())
			s.initedCallback(SourceName)
		}()

		// If wait time is set, we will call the initedCallback after wait time.
		if s.args.WaitTime > 0 {
			go func() {
				time.Sleep(time.Duration(s.args.WaitTime))
				s.initedCallback(SourceName)
			}()
		}
	}

	go func() { // do recon
		reconCh := make(chan struct{}, 1)
		reconCh <- struct{}{}
		starter := &sync.Once{}

		for {
			select {
			case <-s.stop:
				return
			case <-reconCh:
				log.Infof("recv signal, will call reConFunc")
				s.reConFunc(reconCh)
				starter.Do(func() {
					log.Infof("zk connected, will start fetch-data logic")
					if s.isPollingMode() {
						go s.Polling()
					} else {
						go s.Watching()
					}
				})
			}
		}
	}()

	go func() {
		select {
		case <-s.stop:
			return
		case <-s.seInitCh:
			if s.seMergePortMocker != nil {
				log.Infof("%s service entry init done, begin to do init se merge port refresh", SourceName)
				s.seMergePortMocker.Refresh()
				s.initWg.Done()
			}

			if s.args.EnableDubboSidecar {
				log.Infof("%s service entry init done, begin to do init sidecar refresh", SourceName)
				s.refreshSidecar(true)
				s.markSidecarInitDone()
			}
		}

		if s.args.EnableDubboSidecar {
			go s.refreshSidecarTask(s.stop)
		}
		if s.seMergePortMocker != nil {
			go s.seMergePortMocker.Start(s.stop)
		}
	}()
}

func (s *Source) cacheInfo(iface string) interface{} {
	info := make(map[string]interface{}, 0)
	if iface == "" {
		return s.cache
	}

	cacheMap := s.cache.Items()
	for service, seCache := range cacheMap {
		if service == iface {
			info[service] = seCache
			break
		} else {
			for serviceKey, value := range seCache.Items() {
				if serviceKey == iface {
					info[serviceKey] = value
					break
				}
			}
		}
	}
	return info
}

func (s *Source) cacheSummary() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	count := 0
	cacheMap := s.cache.Items()
	for _, ses := range cacheMap {
		for serviceKey, sem := range ses.Items() {
			info[serviceKey] = len(sem.ServiceEntry.Endpoints)
			count = count + 1
		}
	}

	info["count-iface"] = count
	return info
}

func (s *Source) Stop() {
	s.stop <- struct{}{}
}

func (s *Source) ServiceEntries() []*networkingapi.ServiceEntry {
	cacheItems := s.cache.Items()
	ret := make([]*networkingapi.ServiceEntry, 0, len(cacheItems))

	for _, ses := range cacheItems {
		for _, sem := range ses.Items() {
			ret = append(ret, sem.ServiceEntry)
		}
	}

	return ret
}

func (s *Source) ServiceEntry(fullName resource.FullName) *networkingapi.ServiceEntry {
	// here we do not use the ns according to the cache layout.
	serviceKey := string(fullName.Name)
	service := parseServiceFromKey(serviceKey)

	ses, ok := s.cache.Get(service)
	if !ok {
		return nil
	}

	sem, ok := ses.Get(serviceKey)
	if !ok {
		return nil
	}

	return sem.ServiceEntry
}

func (s *Source) markServiceEntryInitDone() {
	s.mut.RLock()
	ch := s.seInitCh
	s.mut.RUnlock()
	if ch == nil {
		return
	}

	s.mut.Lock()
	ch, s.seInitCh = s.seInitCh, nil
	s.mut.Unlock()
	if ch != nil {
		log.Infof("%s service entry init done, close ch and call initWg.Done", SourceName)
		s.initWg.Done()
		close(ch)
	}
}

func (s *Source) onConfig(args *bootstrap.ZookeeperSourceArgs) {
	var prevArgs *bootstrap.ZookeeperSourceArgs
	prevArgs, s.args = s.args, args // XXX should be atomic

	var (
		updateDetails     error
		shouldForceUpdate bool
	)
	s.mut.Lock()
	if !reflect.DeepEqual(prevArgs.EndpointSelectors, args.EndpointSelectors) ||
		!reflect.DeepEqual(prevArgs.ServicedEndpointSelectors, args.ServicedEndpointSelectors) {
		s.instanceFilter = generateInstanceFilter(args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll, args.AlwaysUseSourceScopedEpSelectors)
		updateDetails = multierror.Append(updateDetails, fmt.Errorf("instance filter updated, prev %+v, cur %+v", prevArgs.EndpointSelectors, args.EndpointSelectors))
		shouldForceUpdate = true
	}

	if selectors := args.MethodLBServiceSelectors; !reflect.DeepEqual(prevArgs.MethodLBServiceSelectors, selectors) {
		s.methodLBChecker = generateMethodLBChecker(selectors)
		updateDetails = multierror.Append(updateDetails, fmt.Errorf("method lb checker updated, prev %+v, cur %+v", prevArgs.MethodLBServiceSelectors, selectors))
		shouldForceUpdate = true
	}
	s.mut.Unlock()

	if updateDetails != nil {
		log.Infof("zk source config change details: %s, shouldForceUpdate: %v", updateDetails.Error(), shouldForceUpdate)
	}
	if shouldForceUpdate {
		s.forceUpdate()
	}
}

func generateMethodLBChecker(selectors []*bootstrap.ServiceSelector) func(*convertedServiceEntry) bool {
	var blackListCnt int
	for _, v := range selectors {
		if v.Invert {
			blackListCnt++
		}
	}
	if blackListCnt > 0 && blackListCnt != len(selectors) {
		var whiteList []*bootstrap.ServiceSelector
		for _, v := range selectors {
			if !v.Invert {
				whiteList = append(whiteList, v)
			}
		}
		selectors = whiteList
		blackListCnt = 0
	}

	return func(cse *convertedServiceEntry) bool {
		if len(selectors) == 0 {
			return false
		}

		for _, selector := range selectors {
			if selector.LabelSelector != nil {
				ls, err := metav1.LabelSelectorAsSelector(selector.LabelSelector)
				if err != nil {
					// ignore invalid LabelSelector
					continue
				}

				if ls.Matches(k8slabels.Set(cse.labels)) {
					return blackListCnt == 0 // match, return true for white list, false for black list
				}
			}
		}

		return blackListCnt > 0 // no match, return false for white list, true for black list
	}
}

// handleServiceDelete clean the service entry under the interface node except the ignored ones
// we do not really delete the service entry, but update it with empty endpoints,
// so that the contorl plane can handle it as a EDS event
func (s *Source) handleServiceDelete(iface string, ignoredSes frameworkutil.Set[string]) {
	// if enable empty protection, we don't need to handle interface node delete
	if s.args.EnableEmptyProtection {
		return
	}
	if ses, ok := s.cache.Get(iface); ok {
		for se, sem := range ses.Items() {
			if ignoredSes.Contains(se) {
				continue
			}
			// del event -> empty-ep update event
			if len(sem.ServiceEntry.Endpoints) == 0 {
				continue
			}
			seValueCopy := *sem
			seCopy := *sem.ServiceEntry
			seCopy.Endpoints = make([]*networkingapi.WorkloadEntry, 0)
			seValueCopy.ServiceEntry = &seCopy
			ses.Set(se, &seValueCopy)
			sem = &seValueCopy
			event, err := buildServiceEntryEvent(event.Updated, sem.ServiceEntry, sem.Meta, false)
			if err == nil {
				log.Infof("delete(update) zk se, hosts: %s, ep size: %d ", sem.ServiceEntry.Hosts[0], len(sem.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(event)
				}
			} else {
				log.Errorf("delete(update) svc %s failed, case: %v", se, err.Error())
			}
			monitoring.RecordServiceEntryDeletion(SourceName, false, err == nil)
		}
	}
}

func (s *Source) handleServiceData(providers, consumers, configutators []string, dubboInterface string) {
	if _, ok := s.cache.Get(dubboInterface); !ok {
		s.cache.Set(dubboInterface, cmap.New[*ServiceEntryWithMeta]())
	}

	opts := &convertOptions{
		svcPort:               s.args.SvcPort,
		instancePortAsSvcPort: s.args.InstancePortAsSvcPort,
		patchLabel:            s.args.LabelPatch,
		ignoreLabels:          s.ignoreLabelsMap,
		hostSuffix:            s.args.HostSuffix,
		filter:                s.getInstanceFilter(),
	}
	opts.protocol, opts.protocolName = source.ProtocolName(s.args.SvcProtocol, s.args.GenericProtocol)
	freshSvcMap, freshSeMap := s.convertServiceEntry(providers, consumers, configutators, dubboInterface, opts)
	s.updateRegistryServiceCache(dubboInterface, freshSvcMap)
	s.updateSeCache(freshSeMap, dubboInterface)
}

func (s *Source) updateRegistryServiceCache(dubboInterface string, freshSvcMap map[string][]dubboInstance) {
	if _, ok := s.registryServiceCache.Get(dubboInterface); !ok {
		s.registryServiceCache.Set(dubboInterface, cmap.New[[]dubboInstance]())
	}
	for serviceKey, instances := range freshSvcMap {
		svcCache, ok := s.registryServiceCache.Get(dubboInterface)
		if !ok {
			continue
		}
		svcCache.Set(serviceKey, instances)
	}

	// check if svc deleted
	deleteKey := make([]string, 0)
	svcCache, ok := s.registryServiceCache.Get(dubboInterface)
	if !ok {
		return
	}

	for serviceKey := range svcCache.Items() {
		_, exist := freshSvcMap[serviceKey]
		if exist {
			continue
		}
		deleteKey = append(deleteKey, serviceKey)
	}

	for _, key := range deleteKey {
		svcCache.Remove(key)
	}
}

func (s *Source) updateSeCache(freshSeMap map[string]*convertedServiceEntry, dubboInterface string) {
	if _, ok := s.cache.Get(dubboInterface); !ok {
		s.cache.Set(dubboInterface, cmap.New[*ServiceEntryWithMeta]())
	}

	for serviceKey, convertedSe := range freshSeMap {
		se := convertedSe.se
		now := time.Now()

		meta := resource.Metadata{
			FullName:   resource.FullName{Namespace: resource.Namespace(s.args.ResourceNs), Name: resource.LocalName(serviceKey)},
			CreateTime: now,
			Version:    resource.Version(now.String()),
			Labels: map[string]string{
				"path":     dubboInterface,
				"registry": "zookeeper",
			},
			Annotations: map[string]string{},
		}

		for k, v := range convertedSe.labels {
			meta.Labels[k] = v
		}

		newSeWithMeta, _ := prepareServiceEntryWithMeta(se, meta)

		s.mut.Lock()
		s.serviceMethods[serviceKey] = convertedSe.methodsLabel
		s.mut.Unlock()

		interfaceSeCache, ok := s.cache.Get(dubboInterface)
		if !ok {
			continue
		}

		var preCallModel map[string]DubboCallModel
		interfaceName := se.Hosts[0]
		if s.args.HostSuffix != "" {
			interfaceName = strings.TrimSuffix(interfaceName, s.args.HostSuffix)
		}
		callModel := convertDubboCallModel(se, interfaceName, convertedSe.InboundEndPoints)
		s.mut.Lock()
		preCallModel, s.seDubboCallModels[meta.FullName] = s.seDubboCallModels[meta.FullName], callModel
		changedApps := calcChangedApps(preCallModel, callModel)
		attachCallModel := len(changedApps) > 0
		if attachCallModel {
			if s.changedApps == nil {
				s.changedApps = map[string]struct{}{}
			}
			for _, app := range changedApps {
				s.changedApps[app] = struct{}{}
			}
		}
		s.mut.Unlock()

		if oldSem, exist := interfaceSeCache.Get(serviceKey); !exist {
			interfaceSeCache.Set(serviceKey, newSeWithMeta)
			ev, err := buildServiceEntryEvent(event.Added, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, attachCallModel)
			if err == nil {
				log.Infof("add zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
			monitoring.RecordServiceEntryCreation(SourceName, err == nil)
		} else {
			if oldSem.Equals(*newSeWithMeta) && !attachCallModel {
				continue
			}
			interfaceSeCache.Set(serviceKey, newSeWithMeta)
			ev, err := buildServiceEntryEvent(event.Updated, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, attachCallModel)
			if err == nil {
				log.Infof("update zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
			monitoring.RecordServiceEntryUpdate(SourceName, err == nil)
		}
	}

	ignoredSes := frameworkutil.NewSet[string]()
	for seKey := range freshSeMap {
		ignoredSes.Insert(seKey)
	}
	s.handleServiceDelete(dubboInterface, ignoredSes)
}

func (s *Source) getInstanceFilter() func(*dubboInstance) bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.instanceFilter
}

func generateInstanceFilter(
	svcSel map[string][]*bootstrap.EndpointSelector,
	epSel []*bootstrap.EndpointSelector,
	emptySelectorsReturn bool,
	alwaysUseSourceScopedEpSelectors bool,
) func(*dubboInstance) bool {
	cfgs := make(map[string]source.HookConfig, len(svcSel))
	for svc, selectors := range svcSel {
		cfgs[svc] = source.ConvertEndpointSelectorToHookConfig(selectors, source.HookConfigWithEmptySelectorsReturn(emptySelectorsReturn))
	}
	cfgs[defaultServiceFilter] = source.ConvertEndpointSelectorToHookConfig(epSel, source.HookConfigWithEmptySelectorsReturn(emptySelectorsReturn))
	hookStore := source.NewHookStore(cfgs)
	return func(i *dubboInstance) bool {
		param := source.NewHookParam(
			source.HookParamWithLabels(i.Metadata),
			source.HookParamWithIP(i.Addr),
		)
		filter := hookStore[i.Service]
		if filter == nil {
			filter = hookStore[defaultServiceFilter]
			return filter(param)
		}
		if alwaysUseSourceScopedEpSelectors {
			sourceScopedFilter := hookStore[defaultServiceFilter]
			return sourceScopedFilter(param) && filter(param)
		}
		return filter(param)
	}
}

func (s *Source) forceUpdate() {
	s.mut.Lock()
	forceUpdateTrigger := s.forceUpdateTrigger.Load().(chan struct{})
	s.forceUpdateTrigger.Store(make(chan struct{}))
	s.mut.Unlock()
	close(forceUpdateTrigger)
}

// prepareServiceEntryWithMeta prepare service entry with meta. Will perform cloning to obtain unrelated copies of the
// data, and the event handlers can safely modify its contents.
// In addition, certain metadata will also be populated.
// Returns the prepared service entry with meta and whether the data has been changed.
func prepareServiceEntryWithMeta(se *networkingapi.ServiceEntry, meta resource.Metadata) (*ServiceEntryWithMeta, bool) {
	se = util.CopySe(se)
	meta = meta.Clone()

	var changed bool
	if source.FillRevision(&meta) {
		changed = true
	}
	if util.FillSeLabels(se, &meta) {
		changed = true
	}

	return &ServiceEntryWithMeta{
		ServiceEntry: se,
		Meta:         meta,
	}, changed
}

// buildServiceEntryEvent assembled the incoming data into an event. Event handle should not modify the data.
func buildServiceEntryEvent(kind event.Kind, se *networkingapi.ServiceEntry, meta resource.Metadata, attachCallModel bool) (event.Event, error) {
	return event.Event{
		Kind:   kind,
		Source: collections.ServiceEntry,
		Resource: &resource.Instance{
			Metadata:    meta,
			Message:     se,
			Attachments: map[string]interface{}{AttachmentDubboCallModel: attachCallModel},
		},
	}, nil
}

// buildSidecarEvent assembled the incoming data into an event. Event handle should not modify the data.
func buildSidecarEvent(kind event.Kind, item *networkingapi.Sidecar, meta resource.Metadata) event.Event {
	meta = meta.Clone()
	source.FillRevision(&meta)
	return event.Event{
		Kind:   kind,
		Source: collections.Sidecar,
		Resource: &resource.Instance{
			Metadata: meta,
			Message:  item,
		},
	}
}
