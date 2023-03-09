/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package zookeeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-zookeeper/zk"
	cmap "github.com/orcaman/concurrent-map"
	"istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/pkg/log"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/util"
)

var scope = log.RegisterScope("zk", "zk debugging", 0)

const (
	ZK                        = "zk"
	ZkPath                    = "/zk"
	ZkSimplePath              = "/zks"
	DubboCallModelPath        = "/dubboCallModel"
	SidecarDubboCallModelPath = "/sidecarDubboCallModel"
	ConsumerNode              = "consumers"
	ProviderNode              = "providers"
	Polling                   = "polling"

	AttachmentDubboCallModel = "ATTACHEMT_DUBBO_CALL_MODEL"
)

type Source struct {
	args                        *bootstrap.ZookeeperSourceArgs
	delay                       time.Duration
	addresses                   []string
	timeout                     time.Duration
	refreshPeriod               time.Duration
	RegisterRootNode            string
	ApplicationRegisterRootNode string
	exceptedResources           []collection.Schema
	mode                        string
	zkGatewayModel              bool
	patchLabel                  bool
	ignoreLabels                map[string]string
	watchingRoot                bool // TODO useless?
	watchingWorkerCount         int

	serviceCache         map[string]*ServiceEntryWithMeta
	cache                cmap.ConcurrentMap
	pollingCache         cmap.ConcurrentMap
	sidecarCache         map[resource.FullName]SidecarWithMeta
	dubboCallModels      map[string]DubboCallModel // can only be replaced rather than being modified
	seDubboCallModels    map[resource.FullName]map[string]DubboCallModel
	changedApps          map[string]struct{}
	appSidecarUpdateTime map[string]time.Time
	dubboPortsCache      map[uint32]*v1alpha3.Port

	Con            *atomic.Value // store *zk.Conn
	handlers       []event.Handler
	initedCallback func(string)
	mut            sync.RWMutex

	seInitCh                               chan struct{}
	initWg                                 sync.WaitGroup
	refreshSidecarNotifyCh                 chan struct{}
	refreshSidecarMockServiceEntryNotifyCh chan struct{}
	stop                                   chan struct{}
}

func NewSource(args *bootstrap.ZookeeperSourceArgs, exceptedResources []collection.Schema, delay time.Duration, readyCallback func(string)) (event.Source, func(http.ResponseWriter, *http.Request), func(http.ResponseWriter, *http.Request), error) {
	ignoreLabels := make(map[string]string, 0)
	for _, v := range args.IgnoreLabel {
		ignoreLabels[v] = v
	}

	ret := &Source{
		args:                        args,
		delay:                       delay,
		addresses:                   args.Address,
		timeout:                     time.Duration(args.ConnectionTimeout),
		refreshPeriod:               time.Duration(args.RefreshPeriod),
		mode:                        args.Mode,
		watchingWorkerCount:         args.WatchingWorkerCount,
		patchLabel:                  args.LabelPatch,
		RegisterRootNode:            args.RegistryRootNode,
		ApplicationRegisterRootNode: args.ApplicationRegisterRootNode,
		exceptedResources:           exceptedResources,
		zkGatewayModel:              args.GatewayModel,
		ignoreLabels:                ignoreLabels,
		initedCallback:              readyCallback,

		cache:                cmap.New(),
		pollingCache:         cmap.New(),
		seDubboCallModels:    map[resource.FullName]map[string]DubboCallModel{},
		appSidecarUpdateTime: map[string]time.Time{},
		dubboPortsCache:      map[uint32]*v1alpha3.Port{},

		seInitCh:               make(chan struct{}),
		stop:                   make(chan struct{}),
		watchingRoot:           false,
		refreshSidecarNotifyCh: make(chan struct{}, 1),

		Con: &atomic.Value{},
	}
	ret.handlers = append(
		ret.handlers,
		event.HandlerFromFn(ret.serviceEntryHandlerRefreshSidecar),
		event.HandlerFromFn(ret.serviceEntryHandlerRefreshSidecarMockServiceEntry))

	ret.initWg.Add(1) // ServiceEntry init-sync ready
	if args.EnableDubboSidecar {
		ret.initWg.Add(1) // Sidecar init-sync ready
	}

	return ret, ret.cacheJson, ret.simpleCacheJson, nil
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
		con, _, err := zk.Connect(s.addresses, s.timeout, zk.WithEventCallback(func(ev zk.Event) {
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
			scope.Infof("re connect zk error %v", err)
			time.Sleep(time.Second)
		} else {
			// replace the connection
			s.Con.Store(con)
			break
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
	return s.mode == Polling
}

func (s *Source) Start() {
	if s.initedCallback != nil {
		go func() {
			s.initWg.Wait()
			s.initedCallback(ZK)
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

	if s.args.EnableDubboSidecar {
		go func() {
			select {
			case <-s.stop:
				return
			case <-s.seInitCh:
				s.refreshSidecarMockServiceEntry()
				s.refreshSidecar(true)

				s.markSidecarInitDone()
			}

			go s.refreshSidecarTask(s.stop)
			go func() {
				for {
					select {
					case <-s.stop:
						return
					case <-s.refreshSidecarMockServiceEntryNotifyCh:
						s.refreshSidecarMockServiceEntry()
					}
				}
			}()
		}()
	}
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
	if s.mode == Polling {
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

func (s *Source) ServiceEntries() []*v1alpha3.ServiceEntry {
	cacheItems := s.cacheInUse().Items()
	ret := make([]*v1alpha3.ServiceEntry, 0, len(cacheItems))

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

func (s *Source) ServiceEntry(fullName resource.FullName) *v1alpha3.ServiceEntry {
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
		log.Infof("service entry init done, close ch and call initWg.Done")
		s.initWg.Done()
		close(ch)
	}
}

func buildSeEvent(kind event.Kind, item *v1alpha3.ServiceEntry, meta resource.Metadata, service string, callModel map[string]DubboCallModel) (event.Event, error) {
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

func buildSidecarEvent(kind event.Kind, item *v1alpha3.Sidecar, meta resource.Metadata) event.Event {
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
