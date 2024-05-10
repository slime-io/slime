// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
	"istio.io/libistio/pkg/config/event"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/mcpoverxds"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/multicluster"
	"slime.io/slime/modules/meshregistry/pkg/source"
	utilcache "slime.io/slime/modules/meshregistry/pkg/util/cache"
)

// Processing component is the main config processing component that will listen to a config source and publish
// resources through an MCP server, or a dialout connection.
type Processing struct {
	regArgs      *bootstrap.RegistryArgs
	addOnRegArgs func(onRegArgs func(args *bootstrap.RegistryArgs))

	localCLusterID string

	serveWG       sync.WaitGroup
	listenerMutex sync.Mutex
	listener      net.Listener
	stopCh        chan struct{}
	httpServer    *HttpServer
}

// NewProcessing returns a new processing component.
func NewProcessing(args *Args) *Processing {
	p := &Processing{
		regArgs:        args.RegistryArgs,
		addOnRegArgs:   args.AddOnRegArgs,
		stopCh:         make(chan struct{}),
		localCLusterID: args.RegistryArgs.K8S.ClusterID,
	}
	hs := &HttpServer{
		addr:            args.RegistryArgs.HTTPServerAddr,
		mux:             http.NewServeMux(),
		sourceReady:     true,
		sources:         cmap.New[bool](),
		httpPathHandler: args.SlimeEnv.HttpPathHandler,
	}
	hs.HandleFunc("/args", p.cacheRegArgs)
	hs.HandleFunc("/ready", hs.handleReadyProbe)
	hs.HandleFunc("/clients", hs.handleClientsInfo)
	hs.HandleFunc("/pc", hs.pc)
	hs.HandleFunc("/nc", hs.nc)
	if rm := args.SlimeEnv.ReadyManager; rm != nil {
		rm.AddReadyChecker("ready", hs.ready)
	}
	p.httpServer = hs
	return p
}

// Start implements process.Component
func (p *Processing) Start() (err error) {
	var srcPreStartHooks []func()
	clusterCache := false
	csrc := make([]event.Source, 0, len(source.RegistrySources()))
	for registryID, initlizer := range source.RegistrySources() {
		src, handlers, cacheCluster, skip, err := initlizer(p.regArgs, p.httpServer.SourceReadyCallBack, p.addOnRegArgs)
		if err != nil {
			log.Errorf("init registry source %s failed: %v", registryID, err)
			return err
		}
		if skip {
			continue
		}
		p.httpServer.SourceRegistry(registryID)
		for path, handler := range handlers {
			p.httpServer.HandleFunc(path, handler)
		}
		clusterCache = clusterCache || cacheCluster
		csrc = append(csrc, src)
	}

	if clusterCache {
		srcPreStartHooks = append(srcPreStartHooks, p.initMulticluster())
	}

	if p.addOnRegArgs != nil {
		p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
			p.regArgs = args
		})
	}

	p.httpServer.start()
	// TODO start sources
	mcpController, err := mcpoverxds.NewController(p.regArgs)
	if err != nil {
		log.Errorf("init mcpoverxds controller error: %v", err)
	} else {
		for _, src := range csrc {
			src.Dispatch(mcpController.Handler)
		}
	}

	startWG := &sync.WaitGroup{}
	startWG.Add(1)

	p.serveWG.Add(1)
	p.httpServer.ListenerRegistry(mcpController, startWG, p.startXdsOverMcp)

	go func() {
		for _, hook := range srcPreStartHooks {
			if hook != nil {
				hook()
			}
		}
		monitoring.RecordEnabledSource(len(csrc))
		for _, src := range csrc {
			src.Start()
		}
	}()

	return nil
}

func (p *Processing) startXdsOverMcp(mcpController *mcpoverxds.McpController, _ *sync.WaitGroup) {
	var prevReady bool
	p.httpServer.lock.Lock()
	prevReady, p.httpServer.xdsReady = p.httpServer.xdsReady, true
	p.httpServer.lock.Unlock()

	if prevReady {
		return
	}
	mcpController.Run()
	p.httpServer.HandleFunc("/xdsCache", mcpController.HandleXdsCache)
}

func CombineSources(c []event.Source) event.Source {
	if len(c) == 0 {
		return nil
	}
	o := c[0]
	for i, e := range c {
		if i == 0 {
			continue
		}
		o = event.CombineSources(o, e)
	}
	return o
}

func (p *Processing) getDeployKubeClient() (k kubernetes.Interface, err error) {
	return p.getKubeClient(p.regArgs.K8S.KubeRestConfig, p.regArgs.K8S.ApiServerUrlForDeploy, p.regArgs.K8S.KubeConfig)
}

func (p *Processing) getKubeClient(
	config *rest.Config,
	masterUrl string,
	kubeconfigPath string,
) (k kubernetes.Interface, err error) {
	if config == nil {
		config, err = clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(config)
}

// Stop implements process.Component
func (p *Processing) Stop() {
	if p.stopCh != nil {
		close(p.stopCh)
		p.stopCh = nil
	}

	p.listenerMutex.Lock()
	if p.listener != nil {
		_ = p.listener.Close()
		p.listener = nil
	}
	p.listenerMutex.Unlock()
}

func (p *Processing) getListener() net.Listener {
	p.listenerMutex.Lock()
	defer p.listenerMutex.Unlock()
	return p.listener
}

// Address returns the Address of the MCP service.
func (p *Processing) Address() net.Addr {
	l := p.getListener()
	if l == nil {
		return nil
	}
	return l.Addr()
}

func (p *Processing) initMulticluster() func() {
	k, err := p.getDeployKubeClient()
	if err != nil {
		log.Errorf("get KubeInterfaces %v", err)
		return nil
	}

	watchedNs, localClusterID := p.regArgs.K8S.ClusterRegistriesNamespace, p.localCLusterID
	controller := multicluster.NewController(k, watchedNs, localClusterID, time.Duration(p.regArgs.ResyncPeriod))
	if controller != nil {
		controller.AddHandler(utilcache.K8sPodCaches)
		controller.AddHandler(utilcache.K8sNodeCaches)
		if err = controller.Run(p.stopCh); err != nil {
			log.Errorf("start multicluster controller met err %v", err)
		}
	}
	checkInterval := 100 * time.Millisecond
	timeout := 10 * time.Second
	if p.regArgs.ResyncPeriod > 0 {
		timeout = 2 * time.Duration(p.regArgs.ResyncPeriod)
	}
	return func() {
		wait.PollImmediate(
			checkInterval,
			timeout,
			func() (done bool, err error) {
				if controller.HasSynced() {
					return true, nil
				}
				return false, nil
			},
		)
	}
}

func (p *Processing) cacheRegArgs(w http.ResponseWriter, _ *http.Request) {
	regArgs := p.regArgs
	b, err := json.MarshalIndent(regArgs, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "unable to marshal config: %v", err)
		return
	}
	_, _ = w.Write(b)
}
