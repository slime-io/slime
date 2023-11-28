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
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-zookeeper/zk"
	cmap "github.com/orcaman/concurrent-map"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "zk")

const (
	SourceName                = "zookeeper"
	ZkPath                    = "/zk"
	ZkSimplePath              = "/zks"
	DubboCallModelPath        = "/dubboCallModel"
	SidecarDubboCallModelPath = "/sidecarDubboCallModel"
	ConsumerNode              = "consumers"
	ProviderNode              = "providers"
	ConfiguratorNode          = "configurators"
	Polling                   = "polling"

	AttachmentDubboCallModel = "ATTACHMENT_DUBBO_CALL_MODEL"

	defaultServiceFilter = ""
)

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

type Source struct {
	args *bootstrap.ZookeeperSourceArgs

	exceptedResources []collection.Schema
	ignoreLabelsMap   map[string]string
	watchingRoot      bool // TODO useless?

	serviceMethods       map[string]string
	registryServiceCache cmap.ConcurrentMap // string-interface: cmap(string-host: []dubboInstance)
	cache                cmap.ConcurrentMap // string-interface: cmap(string-host: *ServiceEntryWithMeta)
	pollingCache         cmap.ConcurrentMap // string-interface: cmap(string-host: *ServiceEntryWithMeta)
	sidecarCache         map[resource.FullName]SidecarWithMeta
	dubboCallModels      map[string]DubboCallModel // can only be replaced rather than being modified
	seDubboCallModels    map[resource.FullName]map[string]DubboCallModel
	changedApps          map[string]struct{}
	appSidecarUpdateTime map[string]time.Time
	dubboPortsCache      map[uint32]*networking.Port

	handlers       []event.Handler
	initedCallback func(string)
	mut            sync.RWMutex

	seInitCh                               chan struct{}
	initWg                                 sync.WaitGroup
	refreshSidecarNotifyCh                 chan struct{}
	refreshSidecarMockServiceEntryNotifyCh chan struct{}
	stop                                   chan struct{}

	Con               *zkConn
	seMergePortMocker *source.ServiceEntryMergePortMocker

	// instanceFilter fitler which instance of a service should be include
	// Updates are only allowed when the configuration is loaded or reloaded.
	instanceFilter func(*dubboInstance) bool

	forceUpdateTrigger *atomic.Value // store chan struct{}
}

type Option func(s *Source) error

func WithDynamicConfigOption(addCb func(func(*bootstrap.ZookeeperSourceArgs))) Option {
	return func(s *Source) error {
		addCb(s.onConfig)
		return nil
	}
}

func New(args *bootstrap.ZookeeperSourceArgs, exceptedResources []collection.Schema, delay time.Duration, readyCallback func(string), options ...Option) (event.Source, func(http.ResponseWriter, *http.Request), func(http.ResponseWriter, *http.Request), error) {
	// XXX refactor to config
	if zkSrc := args; zkSrc != nil && zkSrc.GatewayModel {
		zkSrc.SvcPort = 80
		zkSrc.InstancePortAsSvcPort = false
	}

	ignoreLabels := make(map[string]string, 0)
	for _, v := range args.IgnoreLabel {
		ignoreLabels[v] = v
	}

	var svcMocker *source.ServiceEntryMergePortMocker
	if args.MockServiceEntryName != "" {
		if args.MockServiceName == "" {
			return nil, nil, nil, fmt.Errorf("args MockServiceName empty but MockServiceEntryName %s", args.MockServiceEntryName)
		}
		svcMocker = source.NewServiceEntryMergePortMocker(
			args.MockServiceEntryName, args.ResourceNs, args.MockServiceName,
			args.MockServiceMergeInstancePort, args.MockServiceMergeServicePort,
			map[string]string{
				"path":     args.MockServiceName,
				"registry": SourceName,
			})
	}

	ret := &Source{
		args:              args,
		exceptedResources: exceptedResources,
		ignoreLabelsMap:   ignoreLabels,

		initedCallback: readyCallback,

		serviceMethods:       map[string]string{},
		registryServiceCache: cmap.New(),
		cache:                cmap.New(),
		pollingCache:         cmap.New(),
		seDubboCallModels:    map[resource.FullName]map[string]DubboCallModel{},
		appSidecarUpdateTime: map[string]time.Time{},
		dubboPortsCache:      map[uint32]*networking.Port{},

		seInitCh:               make(chan struct{}),
		stop:                   make(chan struct{}),
		watchingRoot:           false,
		refreshSidecarNotifyCh: make(chan struct{}, 1),

		Con:                &zkConn{},
		seMergePortMocker:  svcMocker,
		forceUpdateTrigger: &atomic.Value{},
	}
	ret.forceUpdateTrigger.Store(make(chan struct{}))

	ret.handlers = append(
		ret.handlers,
		event.HandlerFromFn(ret.serviceEntryHandlerRefreshSidecar),
	)

	ret.initWg.Add(1) // ServiceEntry init-sync ready
	if args.EnableDubboSidecar {
		ret.initWg.Add(1) // Sidecar init-sync ready
	}
	if ret.seMergePortMocker != nil {
		ret.handlers = append(ret.handlers, ret.seMergePortMocker)
		svcMocker.SetDispatcher(ret.dispatchMergePortsServiceEntry)
		ret.initWg.Add(1) // merge ports se init-sync ready
	}

	ret.instanceFilter = generateInstanceFilter(args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll, args.AlwaysUseSourceScopedEpSelectors)

	for _, op := range options {
		if err := op(ret); err != nil {
			return nil, nil, nil, err
		}
	}

	return ret, ret.cacheJson, ret.simpleCacheJson, nil
}

func (s *Source) dispatchMergePortsServiceEntry(meta resource.Metadata, se *networking.ServiceEntry) {
	prepared, _ := prepareServiceEntryWithMeta(se, meta)
	ev, err := buildServiceEntryEvent(event.Updated, prepared.ServiceEntry, prepared.Meta, nil)
	if err != nil {
		log.Errorf("buildSeEvent met err %v", err)
		return
	}

	for _, h := range s.handlers {
		h.Handle(ev)
	}
}

func (s *Source) reConFunc(reconCh chan<- struct{}) {
	if s.watchingRoot {
		return // ??
	}

	var curConn *zk.Conn
	if v := s.Con.Load(); v != nil {
		curConn = v.(*zk.Conn)
	}
	if curConn != nil {
		curConn.Close()
	}

	for {
		var con *zk.Conn
		var err error
		var once sync.Once
		con, _, err = zk.Connect(s.args.Address, time.Duration(s.args.ConnectionTimeout),
			zk.WithNoRetryHosts(), // https://github.com/slime-io/go-zk/pull/1
			zk.WithEventCallback(func(ev zk.Event) {
				if ev.Type != zk.EventDisconnected {
					return
				}

				// use the reconnection mechanism of the current client first
				time.Sleep(time.Duration(len(s.args.Address)+1) * time.Second)
				switch con.State() {
				case zk.StateConnected, zk.StateHasSession:
					return
				default:
					// ensure that each zk connection triggers only one reconnect
					once.Do(func() {
						select {
						case reconCh <- struct{}{}:
						default:
						}
					})
				}
			}))
		if err != nil {
			log.Infof("re connect zk error %v", err)
			time.Sleep(time.Second)
		} else {
			// TODO: this should be done in go-zk
			connected := false
			for {
				time.Sleep(1500 * time.Millisecond) // Wait for connecting. When go-zk connects to zk, the timeout is one second.
				connState := con.State()
				if connState == zk.StateConnected || connState == zk.StateHasSession {
					connected = true
					break
				}
				if connState != zk.StateConnecting {
					// connect failed
					log.Debugf("wait for connected failed with current state: %s", connState)
					con.Close()
					break
				}
			}
			if connected {
				// replace the connection
				s.Con.Store(con)
				log.Infof("reconnect to zk successfully")
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

func (s *Source) internalCacheJson(w http.ResponseWriter, req *http.Request, cache cmap.ConcurrentMap) {
	temp := cache
	cacheData := map[string]map[string]interface{}{}
	var result interface{}

	interfaceName := req.URL.Query().Get("interfaceName")
	if interfaceName != "" {
		newTemp := cmap.New()
		if value, exist := temp.Get(interfaceName); exist {
			newTemp.Set(interfaceName, value)
		}
		temp = newTemp
	}
	temp.IterCb(func(dubboInterface string, v interface{}) {
		if v == nil {
			return
		}

		inner := v.(cmap.ConcurrentMap)
		if inner == nil {
			return
		}

		interfaceCacheData := cacheData[dubboInterface]
		if interfaceCacheData == nil {
			interfaceCacheData = map[string]interface{}{}
			cacheData[dubboInterface] = interfaceCacheData
		}
		inner.IterCb(func(host string, v interface{}) {
			switch val := v.(type) {
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
	temp := s.cacheInUse()
	registrySvc := req.URL.Query().Get("registry_services")
	if ok, _ := strconv.ParseBool(registrySvc); ok {
		temp = s.registryServiceCache
	}
	s.internalCacheJson(w, req, temp)
}

func (s *Source) isPollingMode() bool {
	return s.args.Mode == Polling
}

func (s *Source) Start() {
	if s.initedCallback != nil {
		t0 := time.Now()
		go func() {
			s.initWg.Wait()
			monitoring.RecordReady(SourceName, t0, time.Now())
			s.initedCallback(SourceName)
		}()
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
					// s.initPush()
					// TODO 服务自省模式
					// go s.doWatchAppication(s.ApplicationRegisterRootNode)
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
		return s.cacheInUse()
	}

	cacheMap := s.cacheInUse().Items()
	for service, seCache := range cacheMap {
		if service == iface {
			info[service] = seCache
			break
		} else {
			if ses, castok := seCache.(cmap.ConcurrentMap); castok {
				for serviceKey, value := range ses.Items() {
					if serviceKey == iface {
						info[serviceKey] = value
						break
					}
				}
			}
		}
	}
	return info
}

func (s *Source) cacheInUse() cmap.ConcurrentMap {
	if s.args.Mode == Polling {
		return s.pollingCache
	} else {
		return s.cache
	}
}

func (s *Source) cacheSummary() map[string]interface{} {
	info := make(map[string]interface{}, 0)
	count := 0
	cacheMap := s.cacheInUse().Items()
	for _, seCache := range cacheMap {
		if ses, castok := seCache.(cmap.ConcurrentMap); castok {
			for serviceKey, value := range ses.Items() {
				if v, ok := value.(*ServiceEntryWithMeta); ok {
					info[serviceKey] = len(v.ServiceEntry.Endpoints)
					count = count + 1
				}
			}
		}
	}

	info["count-iface"] = count
	return info
}

func (s *Source) Stop() {
	s.stop <- struct{}{}
}

func (s *Source) ServiceEntries() []*networking.ServiceEntry {
	cacheItems := s.cacheInUse().Items()
	ret := make([]*networking.ServiceEntry, 0, len(cacheItems))

	for _, seCache := range cacheItems {
		ses, castOk := seCache.(cmap.ConcurrentMap)
		if !castOk {
			continue
		}
		for _, value := range ses.Items() {
			if sem, ok := value.(*ServiceEntryWithMeta); ok {
				ret = append(ret, sem.ServiceEntry)
			}
		}
	}

	return ret
}

func (s *Source) ServiceEntry(fullName resource.FullName) *networking.ServiceEntry {
	// here we do not use the ns according to the cache layout.
	serviceKey := string(fullName.Name)
	service := parseServiceFromKey(serviceKey)

	v, ok := s.cacheInUse().Get(service)
	if !ok {
		return nil
	}

	ses, castOk := v.(cmap.ConcurrentMap)
	if !castOk {
		return nil
	}

	v, ok = ses.Get(serviceKey)
	if !ok {
		return nil
	}

	sem, ok := v.(*ServiceEntryWithMeta)
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
	prevArgs, s.args = s.args, args
	updated := false

	s.mut.Lock()
	if !reflect.DeepEqual(prevArgs.EndpointSelectors, args.EndpointSelectors) ||
		!reflect.DeepEqual(prevArgs.ServicedEndpointSelectors, args.ServicedEndpointSelectors) {
		newInstSel := generateInstanceFilter(args.ServicedEndpointSelectors, args.EndpointSelectors, !args.EmptyEpSelectorsExcludeAll, args.AlwaysUseSourceScopedEpSelectors)
		s.instanceFilter = newInstSel
		updated = true
	}
	s.mut.Unlock()
	if updated {
		s.forceUpdate()
	}
}

func (s *Source) handleServiceData(
	cacheInUse cmap.ConcurrentMap,
	providers, consumers, configutators []string,
	dubboInterface string,
) {
	if _, ok := cacheInUse.Get(dubboInterface); !ok {
		cacheInUse.Set(dubboInterface, cmap.New())
	}

	freshSvcMap, freshSeMap := convertServiceEntry(providers, consumers, configutators, dubboInterface, s)
	s.updateRegistryServiceCache(dubboInterface, freshSvcMap)
	s.updateSeCache(cacheInUse, freshSeMap, dubboInterface)
}

func (s *Source) updateRegistryServiceCache(dubboInterface string, freshSvcMap map[string][]dubboInstance) {
	if _, ok := s.registryServiceCache.Get(dubboInterface); !ok {
		s.registryServiceCache.Set(dubboInterface, cmap.New())
	}
	for serviceKey, instances := range freshSvcMap {
		v, ok := s.registryServiceCache.Get(dubboInterface)
		if !ok {
			continue
		}
		svcCache, ok := v.(cmap.ConcurrentMap)
		if !ok {
			continue
		}
		svcCache.Set(serviceKey, instances)
	}

	// check if svc deleted
	deleteKey := make([]string, 0)
	v, ok := s.registryServiceCache.Get(dubboInterface)
	if !ok {
		return
	}
	svcCache, ok := v.(cmap.ConcurrentMap)
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

func (s *Source) updateSeCache(cacheInUse cmap.ConcurrentMap, freshSeMap map[string]*convertedServiceEntry, dubboInterface string) {
	if _, ok := cacheInUse.Get(dubboInterface); !ok {
		cacheInUse.Set(dubboInterface, cmap.New())
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
		if !convertedSe.methodsEqual {
			// to trigger svc change/full push in istio sidecar when eq -> uneq or uneq -> eq
			meta.Labels[DubboSvcMethodEqLabel] = strconv.FormatBool(convertedSe.methodsEqual)
		}
		newSeWithMeta, _ := prepareServiceEntryWithMeta(se, meta)

		s.mut.Lock()
		s.serviceMethods[serviceKey] = convertedSe.methodsLabel
		s.mut.Unlock()

		v, ok := cacheInUse.Get(dubboInterface)
		if !ok {
			continue
		}
		interfaceSeCache, ok := v.(cmap.ConcurrentMap)
		if !ok {
			continue
		}

		callModel := convertDubboCallModel(se, convertedSe.InboundEndPoints)

		if value, exist := interfaceSeCache.Get(serviceKey); !exist {
			interfaceSeCache.Set(serviceKey, newSeWithMeta)
			ev, err := buildServiceEntryEvent(event.Added, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, callModel)
			if err == nil {
				log.Infof("add zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
			monitoring.RecordServiceEntryCreation(SourceName, err == nil)
		} else if existSeWithMeta, ok := value.(*ServiceEntryWithMeta); ok {
			if existSeWithMeta.Equals(*newSeWithMeta) {
				continue
			}
			interfaceSeCache.Set(serviceKey, newSeWithMeta)
			ev, err := buildServiceEntryEvent(event.Updated, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, callModel)
			if err == nil {
				log.Infof("update zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
			monitoring.RecordServiceEntryUpdate(SourceName, err == nil)
		}
	}

	// check if svc deleted
	deleteKey := make([]string, 0)
	v, ok := cacheInUse.Get(dubboInterface)
	if !ok {
		return
	}
	seCache, ok := v.(cmap.ConcurrentMap)
	if !ok {
		return
	}
	for serviceKey, v := range seCache.Items() {
		_, exist := freshSeMap[serviceKey]
		if exist {
			continue
		}
		deleteKey = append(deleteKey, serviceKey)
		seValue, ok := v.(*ServiceEntryWithMeta)
		if !ok {
			log.Errorf("cast se failed, key: %s", serviceKey)
			continue
		}

		// del event -> empty-ep update event

		if len(seValue.ServiceEntry.Endpoints) == 0 {
			continue
		}

		seValueCopy := *seValue
		seCopy := *seValue.ServiceEntry
		seCopy.Endpoints = make([]*networking.WorkloadEntry, 0)
		seValueCopy.ServiceEntry = &seCopy
		seCache.Set(serviceKey, &seValueCopy)

		ev, err := buildServiceEntryEvent(event.Updated, seValue.ServiceEntry, seValue.Meta, nil)
		if err == nil {
			log.Infof("delete(update) zk se, hosts: %s, ep size: %d ", seValue.ServiceEntry.Hosts[0], len(seValue.ServiceEntry.Endpoints))
			for _, h := range s.handlers {
				h.Handle(ev)
			}
		} else {
			log.Errorf("delete svc failed, case: %v", err.Error())
		}
		monitoring.RecordServiceEntryDeletion(SourceName, false, err == nil)
	}

	for _, key := range deleteKey {
		seCache.Remove(key)
	}
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
func prepareServiceEntryWithMeta(se *networking.ServiceEntry, meta resource.Metadata) (*ServiceEntryWithMeta, bool) {
	se = util.CopySe(se)
	meta = meta.Clone()

	var changed bool
	if source.FillRevision(meta) {
		changed = true
	}
	if util.FillSeLabels(se, meta) {
		changed = true
	}

	return &ServiceEntryWithMeta{
		ServiceEntry: se,
		Meta:         meta,
	}, changed
}

// buildServiceEntryEvent assembled the incoming data into an event. Event handle should not modify the data.
func buildServiceEntryEvent(kind event.Kind, se *networking.ServiceEntry, meta resource.Metadata, callModel map[string]DubboCallModel) (event.Event, error) {
	return event.Event{
		Kind:   kind,
		Source: collections.K8SNetworkingIstioIoV1Alpha3Serviceentries,
		Resource: &resource.Instance{
			Metadata:    meta,
			Message:     se,
			Attachments: map[string]interface{}{AttachmentDubboCallModel: callModel},
		},
	}, nil
}

// buildSidecarEvent assembled the incoming data into an event. Event handle should not modify the data.
func buildSidecarEvent(kind event.Kind, item *networking.Sidecar, meta resource.Metadata) event.Event {
	meta = meta.Clone()
	source.FillRevision(meta)
	return event.Event{
		Kind:   kind,
		Source: collections.K8SNetworkingIstioIoV1Alpha3Sidecars,
		Resource: &resource.Instance{
			Metadata: meta,
			Message:  item,
		},
	}
}
