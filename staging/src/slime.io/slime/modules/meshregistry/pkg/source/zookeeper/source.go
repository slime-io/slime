/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package zookeeper

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	Polling                   = "polling"

	AttachmentDubboCallModel = "ATTACHMENT_DUBBO_CALL_MODEL"
)

type Source struct {
	args *bootstrap.ZookeeperSourceArgs

	exceptedResources []collection.Schema
	ignoreLabelsMap   map[string]string
	watchingRoot      bool // TODO useless?

	serviceCache         map[string]*ServiceEntryWithMeta
	cache                cmap.ConcurrentMap
	pollingCache         cmap.ConcurrentMap
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

	Con               *atomic.Value // store *zk.Conn
	seMergePortMocker *source.ServiceEntryMergePortMocker
}

func New(args *bootstrap.ZookeeperSourceArgs, exceptedResources []collection.Schema, delay time.Duration, readyCallback func(string)) (event.Source, func(http.ResponseWriter, *http.Request), func(http.ResponseWriter, *http.Request), error) {
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

		cache:                cmap.New(),
		pollingCache:         cmap.New(),
		seDubboCallModels:    map[resource.FullName]map[string]DubboCallModel{},
		appSidecarUpdateTime: map[string]time.Time{},
		dubboPortsCache:      map[uint32]*networking.Port{},

		seInitCh:               make(chan struct{}),
		stop:                   make(chan struct{}),
		watchingRoot:           false,
		refreshSidecarNotifyCh: make(chan struct{}, 1),

		Con:               &atomic.Value{},
		seMergePortMocker: svcMocker,
	}

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

	return ret, ret.cacheJson, ret.simpleCacheJson, nil
}

func (s *Source) dispatchMergePortsServiceEntry(meta resource.Metadata, se *networking.ServiceEntry) {
	ev, err := buildSeEvent(event.Updated, se, meta, nil)
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
		con, _, err := zk.Connect(s.args.Address, time.Duration(s.args.ConnectionTimeout),
			zk.WithNoRetryHosts(), // https://github.com/slime-io/go-zk/pull/1
			zk.WithEventCallback(func(ev zk.Event) {
				if ev.Type != zk.EventDisconnected {
					return
				}

				// notify recon
				select {
				case reconCh <- struct{}{}:
				default:
				}
			}))
		if err != nil {
			log.Infof("re connect zk error %v", err)
			time.Sleep(time.Second)
		} else {
			// TODO: this should be done in go-zk
			connected := false
			for {
				time.Sleep(time.Second) // Wait for connecting. When go-zk connects to zk, the timeout is one second.
				connState := con.State()
				if connState == zk.StateConnected || connState == zk.StateHasSession {
					connected = true
					break
				}
				if connState != zk.StateConnecting {
					// connect failed
					break
				}
				// try connecting another zk instance
			}
			if connected {
				// replace the connection
				s.Con.Store(con)
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

func (s *Source) cacheJson(w http.ResponseWriter, req *http.Request) {
	temp := s.cacheInUse()
	all := make(map[string]interface{}, 0)
	if interfaceName := req.URL.Query().Get("interfaceName"); interfaceName != "" {
		if value, exist := temp.Get(interfaceName); exist {
			all["cache"] = value
		}
	} else {
		all["cache"] = temp
		all["serviceCache"] = s.serviceCache
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal zk se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func (s *Source) isPollingMode() bool {
	return s.args.Mode == Polling
}

func (s *Source) Start() {
	if s.initedCallback != nil {
		go func() {
			s.initWg.Wait()
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

func (s *Source) handleServiceData(cacheInUse cmap.ConcurrentMap, provider, consumer []string, service string) {
	if _, ok := cacheInUse.Get(service); !ok {
		cacheInUse.Set(service, cmap.New())
	}

	freshSeMap := convertServiceEntry(
		provider, consumer, service, s.args.SvcPort, s.args.InstancePortAsSvcPort, s.args.LabelPatch,
		s.ignoreLabelsMap, s.args.GatewayModel)
	for serviceKey, convertedSe := range freshSeMap {
		se := convertedSe.se
		now := time.Now()
		newSeWithMeta := &ServiceEntryWithMeta{
			ServiceEntry: se,
			Meta: resource.Metadata{
				FullName:   resource.FullName{Namespace: resource.Namespace(s.args.ResourceNs), Name: resource.LocalName(serviceKey)},
				CreateTime: now,
				Version:    resource.Version(now.String()),
				Labels: map[string]string{
					"path":            service,
					"registry":        "zookeeper",
					dubboParamMethods: convertedSe.methodsLabel,
				},
				Annotations: map[string]string{},
			},
		}

		if !convertedSe.methodsEqual {
			newSeWithMeta.Meta.Labels[DubboSvcMethodEqLabel] = strconv.FormatBool(convertedSe.methodsEqual)
		}

		v, ok := cacheInUse.Get(service)
		if !ok {
			continue
		}
		seCache, ok := v.(cmap.ConcurrentMap)
		if !ok {
			continue
		}

		callModel := convertDubboCallModel(se, convertedSe.InboundEndPoints)

		if value, exist := seCache.Get(serviceKey); !exist {
			seCache.Set(serviceKey, newSeWithMeta)
			if ev, err := buildSeEvent(event.Added, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, callModel); err == nil {
				log.Infof("add zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
		} else if existSeWithMeta, ok := value.(*ServiceEntryWithMeta); ok {
			if existSeWithMeta.Equals(*newSeWithMeta) {
				continue
			}
			seCache.Set(serviceKey, newSeWithMeta)
			if ev, err := buildSeEvent(event.Updated, newSeWithMeta.ServiceEntry, newSeWithMeta.Meta, callModel); err == nil {
				log.Infof("update zk se, hosts: %s, ep size: %d ", newSeWithMeta.ServiceEntry.Hosts[0], len(newSeWithMeta.ServiceEntry.Endpoints))
				for _, h := range s.handlers {
					h.Handle(ev)
				}
			}
		}
	}

	// check if svc deleted
	deleteKey := make([]string, 0)
	v, ok := cacheInUse.Get(service)
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
			continue
		}

		// del event -> empty-ep update event
		seValue.ServiceEntry.Endpoints = make([]*networking.WorkloadEntry, 0)
		ev, err := buildSeEvent(event.Updated, seValue.ServiceEntry, seValue.Meta, nil)
		if err != nil {
			log.Errorf("delete svc failed, case: %v", err.Error())
			continue
		}
		log.Infof("delete(update) zk se, hosts: %s, ep size: %d ", seValue.ServiceEntry.Hosts[0], len(seValue.ServiceEntry.Endpoints))
		for _, h := range s.handlers {
			h.Handle(ev)
		}
	}

	for _, key := range deleteKey {
		seCache.Remove(key)
	}
}

func buildSeEvent(kind event.Kind, item *networking.ServiceEntry, meta resource.Metadata, callModel map[string]DubboCallModel) (event.Event, error) {
	se := util.CopySe(item)
	source.FillRevision(meta)
	util.FillSeLabels(se, meta)
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

func buildSidecarEvent(kind event.Kind, item *networking.Sidecar, meta resource.Metadata) event.Event {
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
