package zookeeper

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/jpillora/backoff"
	cmap "github.com/orcaman/concurrent-map"
	"istio.io/libistio/pkg/config/event"
)

func (s *Source) ServiceNodeDelete(path string) {
	ss := strings.Split(path, "/")
	service := ss[len(ss)-2]
	if seMap, ok := s.cache.Get(service); ok {
		if ses, ok := seMap.(cmap.ConcurrentMap); ok {
			for serviceKey, value := range ses.Items() {
				if se, ok := value.(*ServiceEntryWithMeta); ok {
					if event, err := buildSeEvent(event.Deleted, se.ServiceEntry, se.Meta, service, nil); err == nil {
						for _, h := range s.handlers {
							h.Handle(event)
						}
					}
					ses.Remove(serviceKey)
				}
			}
		}
		s.cache.Remove(service)
	}
}

func (s *Source) EndpointUpdate(provider, consumer []string, path string) {
	s.handleServiceData(s.cache, provider, consumer, strings.Split(path, "/")[2])
}

func (s *Source) Watching() {
	scope.Info("zk source start to watching")
	sw := ServiceWatcher{
		conn:               s.Con,
		rootPath:           s.RegisterRootNode,
		endpointUpdateFunc: s.EndpointUpdate,
		serviceDeleteFunc:  s.ServiceNodeDelete,
		gatewatModel:       s.zkGatewayModel,
		workers:            make([]*worker, s.watchingWorkerCount),
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.stop
		cancel()
	}()
	sw.Start(ctx)
	s.markServiceEntryInitDone()
}

const (
	providerPathSuffix = "/providers"
	consumerPathSuffix = "/consumers"
)

type EventType int

const (
	EventTypeCreate EventType = iota
	EventTypeDelete
)

type ServiceEvent struct {
	path  string
	etype EventType
}

type EndpointWatcherOpts struct {
	conn                *atomic.Value
	endpointUpdateFunc  func(providers, consumers []string, serverPath string)
	serviceDeleteFunc   func(path string)
	gatewatModel        bool
	initCallbackFactory func() func()
}

func NewEndpointWatcher(servicePath string, opts EndpointWatcherOpts) *EndpointWatcher {
	ew := &EndpointWatcher{
		conn:         opts.conn,
		servicePath:  servicePath,
		gatewatModel: opts.gatewatModel,
		handler: func(providers, consumers []string) {
			opts.endpointUpdateFunc(providers, consumers, servicePath)
		},
		serviceDeleteHandler: func() {
			opts.serviceDeleteFunc(servicePath)
		},
		signalExit:   make(chan struct{}),
		exit:         make(chan struct{}),
		initCallback: opts.initCallbackFactory(),
	}
	return ew
}

type EndpointWatcher struct {
	conn *atomic.Value

	// /{root:dubbo}/{service:<service-name>}
	// - /{root:dubbo}/{service:<service-name>}/providers/dubbo://ip:port/{provider_service}}?xxx
	// - /{root:dubbo}/{service:<service-name>}/consumers/consumer://ip[:port]/{consumer_service}?xxx
	servicePath string

	// update by current status
	handler func(providers, consumers []string)

	serviceDeleteHandler func()

	gatewatModel bool

	signalExit, exit chan struct{}

	initCallback func()
}

// non block
func (ew *EndpointWatcher) Start(ctx context.Context) {
	scope.Debugf("zk endpointWatcher %q start watching", ew.servicePath)
	if !ew.gatewatModel {
		go ew.watchBoth(ctx)
	} else {
		go ew.watchProviderOnly(ctx)
	}
}

func (ew *EndpointWatcher) Exit() {
	close(ew.signalExit)
	// wait for exit
	<-ew.exit
}

func (ew *EndpointWatcher) watchProviderOnly(ctx context.Context) {
	defer close(ew.exit)

	// Try to initialize, but it is not required to be completed,
	// because the service may have been deleted or for others.
	// todo: There is a get request and a watch request at startup,
	// if use the returns of the watch request to initialize, the
	// requests at startup can be halved.
	providers, _, err := ew.conn.Load().(*zk.Conn).Children(ew.servicePath + providerPathSuffix)
	if err == nil {
		ew.handler(providers, nil)
	}
	ew.initCallback()

	b := backoff.Backoff{
		Min: 500 * time.Millisecond,
		Max: time.Minute,
	}
	for {
		select {
		case <-ew.signalExit:
			ew.serviceDeleteHandler()
			scope.Infof("endpointWatcher %q's providerOnlyWatching exit due to service deleted", ew.servicePath)
			return
		default:
		}

		select {
		case <-ctx.Done():
			scope.Debugf("endpointWatcher %q's providerOnlyWatching exit due to context close", ew.servicePath)
			return
		default:
		}

		providers, _, eventCh, err := ew.conn.Load().(*zk.Conn).ChildrenW(ew.servicePath + providerPathSuffix)
		if err != nil {
			scope.Warnf("endpointWatcher %q's providerOnlyWatching watch provider failed: %v", ew.servicePath, err)
			time.Sleep(b.Duration())
			continue
		}
		b.Reset()
		ew.handler(providers, nil)
		// todo: If `<-ew.signalExit` or `<-ctx.Done()` happens, we don't know it immediately
		<-eventCh
	}
}

func (ew *EndpointWatcher) simpleWatch(path string, ch chan []string) {
	b := backoff.Backoff{
		Min: 500 * time.Millisecond,
		Max: time.Minute,
	}
	for {
		select {
		case <-ew.exit:
			return
		default:
		}

		paths, _, eventCh, err := ew.conn.Load().(*zk.Conn).ChildrenW(path)
		if err != nil {
			scope.Debugf("endpointWatcher %q watch %q falied: %v", ew.servicePath, path, err)
			time.Sleep(b.Duration())
			continue
		}
		b.Reset()

		select {
		case ch <- paths:
			select {
			case <-ew.exit:
				return
			case <-eventCh:
			}
		case <-ew.exit:
			return
		case <-eventCh:
		}
	}
}

func (ew *EndpointWatcher) watchBoth(ctx context.Context) {
	defer close(ew.exit)

	var providerCache, consumerCache []string

	// Try to initialize, but it is not required to be completed,
	// because the service may have been deleted or for others.
	// todo: There is a get request and a watch request at startup,
	// if use the returns of the watch request to initialize, the
	// requests at startup can be halved.
	providers, _, err := ew.conn.Load().(*zk.Conn).Children(ew.servicePath + providerPathSuffix)
	if err == nil {
		// ignore errors when getting consumer fails
		consumers, _, _ := ew.conn.Load().(*zk.Conn).Children(ew.servicePath + consumerPathSuffix)
		providerCache, consumerCache = providers, consumers
		ew.handler(providerCache, consumerCache)
	}
	ew.initCallback()

	providerEventCh, consumerEventCh := make(chan []string), make(chan []string)
	go ew.simpleWatch(ew.servicePath+providerPathSuffix, providerEventCh)
	go ew.simpleWatch(ew.servicePath+consumerPathSuffix, consumerEventCh)
	for {
		// Delete event highest priority
		select {
		case <-ew.signalExit:
			ew.serviceDeleteHandler()
			scope.Infof("endpointWatcher %q exit due to service deleted", ew.servicePath)
			return
		default:
		}

		select {
		case <-ctx.Done():
			scope.Debugf("endpointWatcher %q exit due to ctx.Done()", ew.servicePath)
			return
		default:
		}

		// todo: If `<-ew.signalExit` or `<-ctx.Done()` happens, we don't know it immediately
		select {
		case providers := <-providerEventCh:
			providerCache = providers
		case consumers := <-consumerEventCh:
			consumerCache = consumers
		}
		ew.handler(providerCache, consumerCache)
	}
}

type ServiceEventQueue interface {
	Push(ServiceEvent)
	// Pop blocks until it can return an item
	Pop() ServiceEvent
}

func NewQueue() ServiceEventQueue {
	return &eventQueue{
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

type eventQueue struct {
	cond *sync.Cond
	q    []ServiceEvent
}

func (q *eventQueue) Push(item ServiceEvent) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	q.q = append(q.q, item)
	q.cond.Signal()
}

func (q *eventQueue) Pop() (item ServiceEvent) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for len(q.q) == 0 {
		q.cond.Wait()
	}
	item = q.q[0]
	q.q[0] = ServiceEvent{}
	q.q = q.q[1:]
	return item
}

func NewServiceWorker(conn *atomic.Value,
	endpointUpdateFunc func([]string, []string, string),
	serviceDeleteFunc func(string),
	gatewatModel bool,
	initCallbackFactory func() func()) *worker {
	return &worker{
		q:     NewQueue(),
		cache: cmap.New(),
		epWatcherOpts: EndpointWatcherOpts{
			conn:                conn,
			endpointUpdateFunc:  endpointUpdateFunc,
			serviceDeleteFunc:   serviceDeleteFunc,
			gatewatModel:        gatewatModel,
			initCallbackFactory: initCallbackFactory,
		},
	}
}

type worker struct {
	q     ServiceEventQueue
	cache cmap.ConcurrentMap

	epWatcherOpts EndpointWatcherOpts
}

// non block
func (w *worker) Start(ctx context.Context) {
	go w.process(ctx)
}

func (w *worker) HandleEvent(e ServiceEvent) {
	w.q.Push(e)
}

func (w *worker) process(ctx context.Context) {
	for w.processItem(ctx) {
	}
}

func (w *worker) processItem(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}
	e := w.q.Pop()
	watcher, ok := w.cache.Get(e.path)
	if e.etype == EventTypeDelete && ok {
		ew := watcher.(*EndpointWatcher)
		ew.Exit() // may block other service events from being processed？
		scope.Infof("endpoint watcher %q exited", ew.servicePath)
		w.cache.Remove(e.path)
		return true
	}
	if e.etype == EventTypeCreate && !ok {
		ew := NewEndpointWatcher(e.path, w.epWatcherOpts)
		w.cache.Set(e.path, ew)
		scope.Infof("service worker start a new endpoint watcher for %s", e.path)
		ew.Start(ctx)
		return true
	}
	return true
}

var dubboExcludeServicePath = []string{
	"metadata", // The application level metadata of zookeeper is located in /dubbo/metadata
	"mapping",  // The interface-application_name mapping information of zookeeper is located in /dubbo/mapping
}

type ServiceWatcher struct {
	ctx context.Context

	conn               *atomic.Value
	rootPath           string
	endpointUpdateFunc func([]string, []string, string)
	serviceDeleteFunc  func(string)
	gatewatModel       bool

	svcs []string

	workers []*worker

	initLock               sync.Mutex
	initWait               sync.WaitGroup
	initCnt, initThreshold int
}

// block until initialized
func (sw *ServiceWatcher) Start(ctx context.Context) {
	sw.ctx = ctx
	initCh := make(chan struct{})
	go func() {
		firstLoop := true
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			paths, _, e, err := sw.conn.Load().(*zk.Conn).ChildrenW(sw.rootPath)
			if err != nil {
				if errors.Is(err, zk.ErrNoNode) {
					// mark ready
					firstLoop = false
					close(initCh)
				}
				continue
			}
			svcs := make([]string, 0, len(paths))
		outter:
			for _, s := range paths {
				for _, exclude := range dubboExcludeServicePath {
					if s == exclude {
						continue outter
					}
				}
				svcs = append(svcs, s)
			}
			if firstLoop {
				firstLoop = false
				// For the zk watching mode, we initialize according to the number of services
				// obtained after registering the watch root node for the first time.
				// When the ep watchers for these services start up and perform their first processing,
				// the zk source is marked as ready.
				initThreshold := len(svcs)
				sw.initThreshold = initThreshold
				sw.initWait.Add(sw.initThreshold)
				go func() {
					sw.initWait.Wait()
					close(initCh)
				}()
			}
			sw.handleSvcs(svcs)

			select {
			case <-ctx.Done():
				return
			case <-e:
			}
		}
	}()
	<-initCh
}

func (sw *ServiceWatcher) handleSvcs(svcs []string) {
	var oldSvcs []string
	sw.svcs, oldSvcs = svcs, sw.svcs
	scope.Debugf("service watcher handle event with current services: %v, last services: %v", sw.svcs, oldSvcs)
	deleted, created := calculateDiff(oldSvcs, sw.svcs)
	scope.Debugf("service watcher calculate diff with delete: %v, create: %v", deleted, created)
	// dispatch deleted service first
	for _, d := range deleted {
		sw.dispatch(ServiceEvent{
			path:  filepath.Join(sw.rootPath, d),
			etype: EventTypeDelete,
		})
	}
	for _, c := range created {
		sw.dispatch(ServiceEvent{
			path:  filepath.Join(sw.rootPath, c),
			etype: EventTypeCreate,
		})
	}
}

func (sw *ServiceWatcher) dispatch(e ServiceEvent) {
	workerIdx := fnv32(e.path) % uint32(len(sw.workers))
	if sw.workers[workerIdx] == nil {
		w := NewServiceWorker(sw.conn, sw.endpointUpdateFunc, sw.serviceDeleteFunc, sw.gatewatModel, sw.initCallbackFactory)
		w.Start(sw.ctx)
		sw.workers[workerIdx] = w
	}
	sw.workers[workerIdx].HandleEvent(e)
}

func (sw *ServiceWatcher) initCallbackFactory() func() {
	sw.initLock.Lock()
	defer sw.initLock.Unlock()
	if sw.initCnt < sw.initThreshold {
		sw.initCnt++
		return sw.initWait.Done
	}
	return func() {}
}

func calculateDiff(o, n []string) (deleted, created []string) {
	mo := make(map[string]struct{}, len(o))
	for _, s := range o {
		mo[s] = struct{}{}
	}
	mn := make(map[string]struct{}, len(n))
	for _, s := range n {
		mn[s] = struct{}{}
	}

	for s := range mo {
		if _, exist := mn[s]; !exist {
			deleted = append(deleted, s)
		}
	}
	for s := range mn {
		if _, exist := mo[s]; !exist {
			created = append(created, s)
		}
	}

	return
}

const (
	offset32 = 2166136261
	prime32  = 16777619
)

// copy from hash/fnv/fnv.go
func fnv32(s string) uint32 {
	h := uint32(offset32)
	for _, c := range s {
		h *= prime32
		h ^= uint32(c)
	}
	return h
}
