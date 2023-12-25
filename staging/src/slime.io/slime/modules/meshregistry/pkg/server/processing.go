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
	"google.golang.org/grpc"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/libistio/pkg/config/schema/resource"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/mcpoverxds"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/multicluster"
	"slime.io/slime/modules/meshregistry/pkg/source/eureka"
	"slime.io/slime/modules/meshregistry/pkg/source/k8s/fs"
	"slime.io/slime/modules/meshregistry/pkg/source/nacos"
	"slime.io/slime/modules/meshregistry/pkg/source/zookeeper"
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

	kubeSourceSchemas *collection.Schemas
}

// NewProcessing returns a new processing component.
func NewProcessing(args *Args) *Processing {
	hs := &HttpServer{
		addr:            args.RegistryArgs.HTTPServerAddr,
		mux:             http.NewServeMux(),
		sourceReady:     true,
		sources:         cmap.New[bool](),
		httpPathHandler: args.SlimeEnv.HttpPathHandler,
	}

	if rm := args.SlimeEnv.ReadyManager; rm != nil {
		rm.AddReadyChecker("ready", hs.ready)
	}

	hs.HandleFunc("/ready", hs.handleReadyProbe)
	hs.HandleFunc("/clients", hs.handleClientsInfo)
	hs.HandleFunc("/pc", hs.pc)
	hs.HandleFunc("/nc", hs.nc)

	ret := &Processing{
		regArgs:        args.RegistryArgs,
		addOnRegArgs:   args.AddOnRegArgs,
		stopCh:         make(chan struct{}),
		httpServer:     hs,
		localCLusterID: args.RegistryArgs.K8S.ClusterID,
	}

	return ret
}

// Start implements process.Component
func (p *Processing) Start() (err error) {
	csrc := make([]event.Source, 0, 5)
	var (
		kubeFsSrc    event.Source
		zookeeperSrc event.Source
		eurekaSrc    event.Source
		nacosSrc     event.Source

		httpHandle       func(http.ResponseWriter, *http.Request)
		simpleHttpHandle func(http.ResponseWriter, *http.Request)
	)

	if p.regArgs.K8SSource.Enabled {
		if p.regArgs.K8SSource.EnableConfigFile && p.regArgs.K8SSource.ConfigPath != "" {
			schemas := p.getKubeSourceSchemas()
			log.Warnf("watching config files in %s", p.regArgs.K8SSource.ConfigPath)
			log.Warnf("watching config schemas %v", schemas.Kinds())
			kubeFsSrc, err = fs.New(p.regArgs.K8SSource.ConfigPath, schemas, p.regArgs.K8SSource.WatchConfigFiles)
			if err != nil {
				return
			}
		}
	}

	clusterCache := false

	if srcArgs := p.regArgs.ZookeeperSource.SourceArgs; srcArgs.Enabled {
		if zookeeperSrc, httpHandle, simpleHttpHandle, err = zookeeper.New(
			p.regArgs.ZookeeperSource, time.Duration(p.regArgs.RegistryStartDelay),
			p.httpServer.SourceReadyCallBack, zookeeper.WithDynamicConfigOption(func(onZookeeperArgs func(*bootstrap.ZookeeperSourceArgs)) {
				if p.addOnRegArgs != nil {
					p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
						onZookeeperArgs(args.ZookeeperSource)
					})
				}
			})); err != nil {
			return
		}
		p.httpServer.HandleFunc(zookeeper.ZkPath, httpHandle)
		p.httpServer.HandleFunc(zookeeper.ZkSimplePath, simpleHttpHandle)
		if zkSrc, ok := zookeeperSrc.(*zookeeper.Source); ok {
			p.httpServer.HandleFunc(zookeeper.DubboCallModelPath, zkSrc.HandleDubboCallModel)
			p.httpServer.HandleFunc(zookeeper.SidecarDubboCallModelPath, zkSrc.HandleSidecarDubboCallModel)
		}
		p.httpServer.SourceRegistry(zookeeper.SourceName)
		if srcArgs.WaitTime > 0 {
			p.httpServer.SourceReadyLater(zookeeper.SourceName, time.Duration(srcArgs.WaitTime))
		}
		clusterCache = clusterCache || p.regArgs.ZookeeperSource.LabelPatch
	}

	if srcArgs := p.regArgs.EurekaSource; srcArgs.Enabled {
		if eurekaSrc, httpHandle, err = eureka.New(p.regArgs.EurekaSource, time.Duration(p.regArgs.RegistryStartDelay), p.httpServer.SourceReadyCallBack); err != nil {
			return
		}
		p.httpServer.HandleFunc(eureka.HttpPath, httpHandle)
		p.httpServer.SourceRegistry(eureka.SourceName)
		if srcArgs.WaitTime > 0 {
			p.httpServer.SourceReadyLater(eureka.SourceName, time.Duration(srcArgs.WaitTime))
		}
		clusterCache = clusterCache || p.regArgs.EurekaSource.LabelPatch
	}

	if srcArgs := p.regArgs.NacosSource; srcArgs.Enabled {
		if nacosSrc, httpHandle, err = nacos.New(
			p.regArgs.NacosSource, srcArgs.NsHost, srcArgs.K8sDomainSuffix, time.Duration(p.regArgs.RegistryStartDelay),
			p.httpServer.SourceReadyCallBack, nacos.WithDynamicConfigOption(func(onNacosArgs func(*bootstrap.NacosSourceArgs)) {
				if p.addOnRegArgs != nil {
					p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
						onNacosArgs(args.NacosSource)
					})
				}
			})); err != nil {
			return
		}
		p.httpServer.HandleFunc(nacos.HttpPath, httpHandle)
		p.httpServer.SourceRegistry(nacos.SourceName)
		if srcArgs.WaitTime > 0 {
			p.httpServer.SourceReadyLater(nacos.SourceName, time.Duration(srcArgs.WaitTime))
		}
		clusterCache = clusterCache || p.regArgs.EurekaSource.LabelPatch
	}

	if clusterCache {
		p.initMulticluster()
	}

	p.httpServer.HandleFunc("/args", p.cacheRegArgs)
	if p.addOnRegArgs != nil {
		p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
			p.regArgs = args
		})
	}

	if kubeFsSrc != nil {
		csrc = append(csrc, kubeFsSrc)
	}
	if zookeeperSrc != nil {
		csrc = append(csrc, zookeeperSrc)
	}
	if eurekaSrc != nil {
		csrc = append(csrc, eurekaSrc)
	}
	if nacosSrc != nil {
		csrc = append(csrc, nacosSrc)
	}

	p.httpServer.start()
	grpc.EnableTracing = p.regArgs.EnableGRPCTracing

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
		monitoring.RecordEnabledSource(len(csrc))
		for _, src := range csrc {
			src.Start()
		}
	}()

	return nil
}

func (p *Processing) startXdsOverMcp(mcpController *mcpoverxds.McpController, startWG *sync.WaitGroup) {
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

func (p *Processing) getKubeClient(config *rest.Config, masterUrl, kubeconfigPath string) (k kubernetes.Interface, err error) {
	if config == nil {
		config, err = clientcmd.BuildConfigFromFlags(p.regArgs.K8S.ApiServerUrl, p.regArgs.K8S.KubeConfig)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(config)
}

func (p *Processing) getKubeSourceSchemas() collection.Schemas {
	if p.kubeSourceSchemas == nil {
		builder := collection.NewSchemasBuilder()

		colMap := make(map[string]struct{})
		for _, col := range p.regArgs.K8SSource.Collections {
			colMap[col] = struct{}{}
		}
		for _, col := range p.regArgs.Snapshots {
			colMap[col] = struct{}{}
		}
		excludeKindMap := make(map[string]struct{})
		for _, k := range p.regArgs.K8SSource.ExcludedResourceKinds {
			excludeKindMap[k] = struct{}{}
		}
		for _, col := range p.regArgs.ExcludedResourceKinds {
			excludeKindMap[col] = struct{}{}
		}
		schemaMap := make(map[resource.Schema]struct{})
		for col := range colMap {
			var schemas collection.Schemas
			switch col {
			case bootstrap.CollectionsAll:
				schemas = collections.All
			case bootstrap.CollectionsIstio:
				schemas = collections.PilotGatewayAPI()
			case bootstrap.CollectionsLegacyDefault:
				schemas = collections.LegacyDefault
			case bootstrap.CollectionsLegacyLocal:
				schemas = collections.LegacyLocalAnalysis
			}
			for _, s := range schemas.All() {
				if _, ok := excludeKindMap[s.Kind()]; ok {
					continue
				}
				schemaMap[s] = struct{}{}
			}
		}
		for s := range schemaMap {
			builder.MustAdd(s)
		}
		schemas := builder.Build()
		p.kubeSourceSchemas = &schemas
	}
	return *p.kubeSourceSchemas
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

func (p *Processing) initMulticluster() {
	k, err := p.getDeployKubeClient()
	if err != nil {
		log.Errorf("get KubeInterfaces %v", err)
		return
	}

	controller := multicluster.NewController(k, p.regArgs.K8S.ClusterRegistriesNamespace, time.Duration(p.regArgs.ResyncPeriod), p.localCLusterID)
	if controller != nil {
		controller.AddHandler(utilcache.K8sPodCaches)
		controller.AddHandler(utilcache.K8sNodeCaches)
		if err = controller.Run(p.stopCh); err != nil {
			log.Errorf("start multicluster controller met err %v", err)
		}
	}
}

func (p *Processing) cacheRegArgs(w http.ResponseWriter, r *http.Request) {
	regArgs := p.regArgs
	b, err := json.MarshalIndent(regArgs, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "unable to marshal config: %v", err)
		return
	}
	_, _ = w.Write(b)
}
