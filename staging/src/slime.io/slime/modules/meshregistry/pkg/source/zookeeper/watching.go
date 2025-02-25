package zookeeper

import (
	"context"
	"errors"
	"math/rand"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/jpillora/backoff"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/robfig/cron/v3"
	"istio.io/libistio/pkg/env"

	"slime.io/slime/framework/util"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

var (
	forceUpdateJitterDuration = env.RegisterDurationVar(
		"FORCE_UPDATE_JITTER_DURATION",
		time.Minute,
		"Jitter time window for doing forced updates, default to 1 minute",
	).Get()

	cronOption = cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor
)

func waitDoForceUpdate() {
	time.Sleep(jitter(forceUpdateJitterDuration))
}

// jitter returns a random duration between 0 and period
func jitter(period time.Duration) time.Duration {
	return time.Duration(float64(period) * rand.Float64())
}

func (s *Source) ServiceNodeDelete(path string) {
	ss := strings.Split(path, "/")
	service := ss[len(ss)-2]
	s.handleServiceDelete(service, util.NewSet[string]())
}

func (s *Source) EndpointUpdate(providers, consumers, configurators []string, path string) {
	s.handleServiceData(providers, consumers, configurators, strings.Split(path, "/")[2])
}

func (s *Source) Watching() {
	log.Info("zk source start to watching")
	sw := ServiceWatcher{
		conn:               s.Con,
		rootPath:           s.args.RegistryRootNode,
		endpointUpdateFunc: s.EndpointUpdate,
		serviceDeleteFunc:  s.ServiceNodeDelete,
		consumerPath:       s.args.ConsumerPath,
		providerPath:       providerPathSuffix,
		watchConfigurators: s.args.EnableConfiguratorMeta,
		workers:            make([]*worker, s.args.WatchingWorkerCount),
		forceUpdateTrigger: s.forceUpdateTrigger,
		forceUpdate:        false,
		debounceConfig:     s.args.WatchingDebounce,
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-s.stop
		cancel()
	}()

	go func() {
		if s.args.WatchingResyncCron == "" {
			return
		}
		log.Infof("watching add rewatch cron job with specs: %q", s.args.WatchingResyncCron)
		c := cron.New(
			cron.WithParser(cron.NewParser(cronOption)),
			cron.WithLogger(cron.VerbosePrintfLogger(log)),
		)
		for _, spec := range strings.Split(s.args.WatchingResyncCron, ",") {
			if spec = strings.TrimSpace(spec); spec != "" {
				_, err := c.AddFunc(spec, func() { s.forceUpdate() })
				if err != nil {
					log.Errorf("watching add rewatch cron job failed, err: %v", err)
				}
			}
		}
		c.Start() // asynchronized run cron job
		<-ctx.Done()
		c.Stop()
	}()

	sw.Start(ctx)
	s.markServiceEntryInitDone()
}

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
	conn                       ZkConn
	endpointUpdateFunc         func(providers, consumers, configurators []string, serverPath string)
	serviceDeleteFunc          func(path string)
	consumerPath, providerPath string
	initCallbackFactory        func() func()
	forceUpdateTrigger         *atomic.Value
	watchConfigurators         bool
	debounceConfig             *bootstrap.WatchingDebounce
}

func NewEndpointWatcher(servicePath string, opts EndpointWatcherOpts) *EndpointWatcher {
	ew := &EndpointWatcher{
		conn:         opts.conn,
		servicePath:  servicePath,
		consumerPath: opts.consumerPath,
		providerPath: opts.providerPath,
		handler: func(providers, consumers, configurators []string) {
			opts.endpointUpdateFunc(providers, consumers, configurators, servicePath)
		},
		serviceDeleteHandler: func() {
			opts.serviceDeleteFunc(servicePath)
		},
		signalExit:         make(chan struct{}),
		exit:               make(chan struct{}),
		initCallback:       opts.initCallbackFactory(),
		watchConfigurators: opts.watchConfigurators,
		debounceConfig:     opts.debounceConfig,
	}
	ew.forceUpdateTrigger = opts.forceUpdateTrigger
	return ew
}

type EndpointWatcher struct {
	conn ZkConn

	// /{root:dubbo}/{service:<service-name>}
	// - /{root:dubbo}/{service:<service-name>}/providers/dubbo://ip:port/{provider_service}}?xxx
	// - /{root:dubbo}/{service:<service-name>}/consumers/consumer://ip[:port]/{consumer_service}?xxx
	servicePath string

	// update by current status
	handler func(providers, consumers, configurators []string)

	serviceDeleteHandler func()

	consumerPath, providerPath string
	watchConfigurators         bool

	signalExit, exit chan struct{}

	initCallback func()

	forceUpdateTrigger *atomic.Value
	debounceConfig     *bootstrap.WatchingDebounce
}

// Start should not block
func (ew *EndpointWatcher) Start(ctx context.Context) {
	log.Debugf("zk endpointWatcher %q start watching", ew.servicePath)
	go ew.watchService(ctx, ew.providerPath, ew.consumerPath)
}

func (ew *EndpointWatcher) Exit() {
	close(ew.signalExit)
	// wait for exit
	<-ew.exit
}

type simpleWatchItem struct {
	data        []string
	err         error
	forceUpdate bool
}

func (ew *EndpointWatcher) simpleWatch(path string, ch chan simpleWatchItem) {
	var (
		b = backoff.Backoff{
			Min: 500 * time.Millisecond,
			Max: time.Minute,
		}
		first       = true
		d           = newDebouncer(ew.debounceConfig)
		forceUpdate = false
	)

	for {
		select {
		case <-ew.exit:
			return
		default:
		}

		// When registering a watch for the first time, no matter whether it is successful or not,
		// we will return the result to the upper layer, so that the upper layer can execute possible
		// callbacks after determining that the first watch has completed.
		paths, eventCh, err := ew.conn.ChildrenW(path)
		if err != nil {
			log.Debugf("endpointWatcher %q watch %q failed: %v", ew.servicePath, path, err)
			if !first {
				time.Sleep(b.Duration())
				continue
			}
		} else {
			b.Reset()
		}
		d.updateLast()

		log.Debugf("endpointWatcher %q watch %q got: %v", ew.servicePath, path, paths)
		forceUpdateTrigger := ew.forceUpdateTrigger.Load().(chan struct{})
		select {
		case ch <- simpleWatchItem{
			data:        paths,
			err:         err,
			forceUpdate: forceUpdate,
		}:
			if first && err != nil { // especially for the first watch failure
				first = false
				// When the err is not nil, the eventCh will be nil, need to redo watch,
				break
			}
			first = false
			select {
			case <-ew.exit:
				return
			case <-eventCh:
				forceUpdate = false
				d.debounce()
			case <-forceUpdateTrigger: // force update
				forceUpdate = true
				waitDoForceUpdate()
			}
		case <-ew.exit:
			return
		case <-eventCh: // frequent change may delay the data(`paths`) distribute
			forceUpdate = false
			d.debounce()
		case <-forceUpdateTrigger: // force update
			forceUpdate = true
			waitDoForceUpdate()
		}
	}
}

// watchService empty path means ignore ...
func (ew *EndpointWatcher) watchService(ctx context.Context, providerPath, consumerPath string) {
	var (
		providerCache, consumerCache, configuratorCache, oldProviderCache []string
		// Try to initialize, but it is not required to be completed,
		// because the service may have been deleted or for others.
		// So we consider both either valid data or fetch-err as init-done.
		providerInit, consumerInit, configuratorInit          = providerPath == "", consumerPath == "", !ew.watchConfigurators //nolint: lll
		initCallBack                                          = ew.initCallback
		providerEventCh, consumerEventCh, configuratorEventCh = make(chan simpleWatchItem), make(chan simpleWatchItem), make(chan simpleWatchItem) //nolint: lll
	)

	defer func() {
		if initCallBack != nil {
			initCallBack()
		}
		close(ew.exit)
	}()

	if providerPath != "" {
		go ew.simpleWatch(ew.servicePath+providerPath, providerEventCh)
	}
	if consumerPath != "" {
		go ew.simpleWatch(ew.servicePath+consumerPath, consumerEventCh)
	}
	if ew.watchConfigurators {
		go ew.simpleWatch(ew.servicePath+configuratorSuffix, configuratorEventCh)
	}

	for {
		// Delete event has the highest priority
		select {
		case <-ew.signalExit:
			ew.serviceDeleteHandler()
			log.Infof("endpointWatcher %q exit due to service deleted", ew.servicePath)
			return
		default:
		}

		select {
		case <-ctx.Done():
			log.Debugf("endpointWatcher %q exit due to ctx.Done()", ew.servicePath)
			return
		default:
		}

		// todo: If `<-ew.signalExit` or `<-ctx.Done()` happens, we don't know it immediately
		select {
		case item := <-providerEventCh:
			providerInit = true
			if item.err == nil {
				oldProviderCache = providerCache
				providerCache = item.data

				if item.forceUpdate {
					deleted, created := calculateDiff(oldProviderCache, providerCache)
					if len(deleted) != 0 || len(created) != 0 {
						log.Infof("endpoint watcher maybe failed with delete: %v, create: %v", deleted, created)
					}
				}
			}
		case item := <-consumerEventCh:
			consumerInit = true
			if item.err == nil {
				consumerCache = item.data
			}
		case item := <-configuratorEventCh:
			configuratorInit = true
			if item.err == nil {
				configuratorCache = item.data
			}
		case <-ew.signalExit:
			ew.serviceDeleteHandler()
			log.Infof("endpointWatcher %q exit due to service deleted", ew.servicePath)
			return
		}

		if providerInit && consumerInit && configuratorInit {
			ew.handler(providerCache, consumerCache, configuratorCache)
			if initCallBack != nil { // not-init -> init
				initCallBack()
				initCallBack = nil
			}
		}
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

func NewServiceWorker(opts EndpointWatcherOpts) *worker {
	return &worker{
		q:             NewQueue(),
		cache:         cmap.New[*EndpointWatcher](),
		epWatcherOpts: opts,
	}
}

type worker struct {
	q     ServiceEventQueue
	cache cmap.ConcurrentMap[string, *EndpointWatcher]

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
	ew, ok := w.cache.Get(e.path)
	if e.etype == EventTypeDelete && ok {
		ew.Exit() // may block other service events from being processed？
		log.Infof("endpoint watcher %q exited", ew.servicePath)
		w.cache.Remove(e.path)
		return true
	}
	if e.etype == EventTypeCreate && !ok {
		ew := NewEndpointWatcher(e.path, w.epWatcherOpts)
		w.cache.Set(e.path, ew)
		log.Infof("service worker start a new endpoint watcher for %s", e.path)
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

	conn                       ZkConn
	rootPath                   string
	endpointUpdateFunc         func([]string, []string, []string, string)
	serviceDeleteFunc          func(string)
	consumerPath, providerPath string
	watchConfigurators         bool

	svcs []string

	workers []*worker

	initLock               sync.Mutex
	initWait               sync.WaitGroup
	initCnt, initThreshold int

	forceUpdateTrigger *atomic.Value
	forceUpdate        bool
	debounceConfig     *bootstrap.WatchingDebounce
}

// block until initialized
func (sw *ServiceWatcher) Start(ctx context.Context) {
	sw.ctx = ctx
	initCh := make(chan struct{})
	go func() {
		d := newDebouncer(sw.debounceConfig)
		firstLoop := true
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			paths, e, err := sw.conn.ChildrenW(sw.rootPath)
			if err != nil {
				if errors.Is(err, zk.ErrNoNode) && firstLoop {
					// mark ready
					firstLoop = false
					close(initCh)
				}
				continue
			}
			d.updateLast()
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
			forceUpdateTrigger := sw.forceUpdateTrigger.Load().(chan struct{})
			select {
			case <-ctx.Done():
				return
			case <-e:
				d.debounce()
			case <-forceUpdateTrigger:
				sw.forceUpdate = true
				waitDoForceUpdate()
			}
		}
	}()
	<-initCh
}

func (sw *ServiceWatcher) handleSvcs(svcs []string) {
	var oldSvcs []string
	sw.svcs, oldSvcs = svcs, sw.svcs
	log.Debugf("service watcher handle event with current services: %v, last services: %v", sw.svcs, oldSvcs)
	deleted, created := calculateDiff(oldSvcs, sw.svcs)
	log.Debugf("service watcher calculate diff with delete: %v, create: %v", deleted, created)
	if sw.forceUpdate {
		sw.forceUpdate = false
		if len(deleted) != 0 || len(created) != 0 {
			log.Infof("service watcher maybe failed with delete: %v, create: %v", deleted, created)
		}
	}
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
		sw.workers[workerIdx] = NewServiceWorker(EndpointWatcherOpts{
			conn:                sw.conn,
			endpointUpdateFunc:  sw.endpointUpdateFunc,
			serviceDeleteFunc:   sw.serviceDeleteFunc,
			consumerPath:        sw.consumerPath,
			providerPath:        sw.providerPath,
			initCallbackFactory: sw.initCallbackFactory,
			forceUpdateTrigger:  sw.forceUpdateTrigger,
			watchConfigurators:  sw.watchConfigurators,
			debounceConfig:      sw.debounceConfig,
		})
		sw.workers[workerIdx].Start(sw.ctx)
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

type debouncer struct {
	disabled             bool
	delay                time.Duration
	maxDelay             time.Duration
	debounceDuration     time.Duration
	debounceTriggerCount int
	currentCount         int
	overflowCount        int
	lastWatchTime        time.Time
}

func newDebouncer(debounce *bootstrap.WatchingDebounce) *debouncer {
	if debounce == nil {
		return &debouncer{
			disabled: true,
		}
	}
	return &debouncer{
		delay:                time.Duration(debounce.Delay),
		maxDelay:             time.Duration(debounce.MaxDelay),
		debounceDuration:     time.Duration(debounce.DebounceDuration),
		debounceTriggerCount: debounce.TriggerCount,
	}
}

func (d *debouncer) updateLast() {
	d.lastWatchTime = time.Now()
}

func (d *debouncer) debounce() {
	if d.disabled {
		return
	}
	if time.Since(d.lastWatchTime) < d.debounceDuration {
		d.currentCount++
		if d.currentCount > d.debounceTriggerCount {
			d.overflowCount++
			d.currentCount = d.debounceTriggerCount
		}
	} else {
		if d.currentCount > 0 {
			d.currentCount--
		}
		d.overflowCount = 0
	}
	if d.currentCount >= d.debounceTriggerCount {
		factor := 1.0 + float64(d.overflowCount)/float64(d.debounceTriggerCount)
		usingDelay := d.delay * time.Duration(factor)
		if usingDelay > d.maxDelay {
			usingDelay = d.maxDelay
		}
		time.Sleep(usingDelay)
	}
}
