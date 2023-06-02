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
	"net"
	"net/http"
	"sync"
	"time"

	cmap "github.com/orcaman/concurrent-map"
	"google.golang.org/grpc"
	"istio.io/libistio/galley/pkg/config/processor/transforms"
	"istio.io/libistio/galley/pkg/config/source/kube"
	"istio.io/libistio/galley/pkg/config/source/kube/apiserver"
	"istio.io/libistio/galley/pkg/config/util/kuberesource"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/schema"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/mcp/snapshot"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	slimebootstrap "slime.io/slime/framework/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/mcpoverxds"
	"slime.io/slime/modules/meshregistry/pkg/multicluster"
	"slime.io/slime/modules/meshregistry/pkg/source/eureka"
	"slime.io/slime/modules/meshregistry/pkg/source/generic"
	genericeureka "slime.io/slime/modules/meshregistry/pkg/source/generic/eureka"
	genericnacos "slime.io/slime/modules/meshregistry/pkg/source/generic/nacos"
	"slime.io/slime/modules/meshregistry/pkg/source/k8s"
	"slime.io/slime/modules/meshregistry/pkg/source/nacos"
	"slime.io/slime/modules/meshregistry/pkg/source/zookeeper"
	utilcache "slime.io/slime/modules/meshregistry/pkg/util/cache"
)

// Processing component is the main config processing component that will listen to a config source and publish
// resources through an MCP server, or a dialout connection.
type Processing struct {
	env          slimebootstrap.Environment
	regArgs      *bootstrap.RegistryArgs
	addOnRegArgs func(onRegArgs func(args *bootstrap.RegistryArgs))

	mcpCache *snapshot.Cache

	k              kube.Interfaces
	localCLusterID string

	serveWG       sync.WaitGroup
	listenerMutex sync.Mutex
	listener      net.Listener
	stopCh        chan struct{}
	httpServer    *HttpServer

	dynConfigController cache.Controller

	sources []event.Source
}

// NewProcessing returns a new processing component.
func NewProcessing(args *Args) *Processing {
	hs := &HttpServer{
		addr:            args.RegistryArgs.HTTPServerAddr,
		mux:             http.NewServeMux(),
		sourceReady:     true,
		sources:         cmap.New(),
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
		meshConfigFileSrc     event.Source
		kubeSrc, extraKubeSrc event.Source
		zookeeperSrc          event.Source
		eurekaSrc             event.Source
		nacosSrc              event.Source

		httpHandle       func(http.ResponseWriter, *http.Request)
		simpleHttpHandle func(http.ResponseWriter, *http.Request)

		shouldInitKubeClient bool
	)

	if p.regArgs.MeshConfigFile != "" && p.regArgs.K8SSource.Enabled {
		if meshConfigFileSrc, err = meshcfgNewFS(p.regArgs.MeshConfigFile); err != nil {
			return
		}
	}

	shouldInitKubeClient = shouldInitKubeClient || p.regArgs.K8SSource.Enabled

	if shouldInitKubeClient {
		if p.k, err = p.getResourceKubeInterfaces(); err != nil {
			return
		}
	}

	m := schema.MustGet()

	transformProviders := transforms.Providers(m)

	// Disable any unnecessary resources, including resources not in configured snapshots
	var colsInSnapshots collection.Names
	for _, c := range m.AllCollectionsInSnapshots(p.regArgs.Snapshots) {
		colsInSnapshots = append(colsInSnapshots, collection.NewName(c))
	}
	kubeResources := kuberesource.DisableExcludedCollections(m.KubeCollections(), transformProviders,
		colsInSnapshots, p.regArgs.ExcludedResourceKinds, p.regArgs.EnableServiceDiscovery)

	if p.regArgs.K8SSource.EnableConfigFile && p.regArgs.K8SSource.ConfigPath != "" {
		if extraKubeSrc, err = p.createFileKubeSource(kubeResources, p.regArgs.K8SSource.ConfigPath, p.regArgs.K8SSource.WatchConfigFiles); err != nil {
			log.Errorf("create extra k8s file source met err %v", err)
			return
		}
	}

	clusterCache := false

	if srcArgs := p.regArgs.ZookeeperSource.SourceArgs; srcArgs.Enabled {
		if zookeeperSrc, httpHandle, simpleHttpHandle, err = zookeeper.New(p.regArgs.ZookeeperSource, kubeResources.All(), time.Duration(p.regArgs.RegistryStartDelay), p.httpServer.SourceReadyCallBack); err != nil {
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
		if eurekaSrc, httpHandle, err = genericeureka.Source(
			p.regArgs.EurekaSource,
			srcArgs.NsHost,
			srcArgs.K8sDomainSuffix,
			time.Duration(p.regArgs.RegistryStartDelay),
			p.httpServer.SourceReadyCallBack,
			generic.WithDynamicConfigOption[*genericeureka.Instance, *genericeureka.Application](func(onArgs func(*bootstrap.SourceArgs)) {
				if p.addOnRegArgs != nil {
					p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
						onArgs(&args.EurekaSource.SourceArgs)
					})
				}
			})); err != nil {
			return
		}
		// if eurekaSrc, httpHandle, err = eureka.New(p.regArgs.EurekaSource, time.Duration(p.regArgs.RegistryStartDelay), p.httpServer.SourceReadyCallBack); err != nil {
		// 	return
		// }
		p.httpServer.HandleFunc(eureka.HttpPath, httpHandle)
		p.httpServer.SourceRegistry(eureka.SourceName)
		if srcArgs.WaitTime > 0 {
			p.httpServer.SourceReadyLater(eureka.SourceName, time.Duration(srcArgs.WaitTime))
		}
		clusterCache = clusterCache || p.regArgs.EurekaSource.LabelPatch
	}

	if srcArgs := p.regArgs.NacosSource; srcArgs.Enabled {
		if nacosSrc, httpHandle, err = genericnacos.Source(
			p.regArgs.NacosSource,
			srcArgs.NsHost,
			srcArgs.K8sDomainSuffix,
			time.Duration(p.regArgs.RegistryStartDelay),
			p.httpServer.SourceReadyCallBack,
			generic.WithDynamicConfigOption[*genericnacos.Instance, *genericnacos.Application](func(onArgs func(*bootstrap.SourceArgs)) {
				if p.addOnRegArgs != nil {
					p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
						onArgs(&args.NacosSource.SourceArgs)
					})
				}
			})); err != nil {
			return
		}
		// if nacosSrc, httpHandle, err = nacos.New(
		// 	p.regArgs.NacosSource, srcArgs.NsHost, srcArgs.K8sDomainSuffix, time.Duration(p.regArgs.RegistryStartDelay),
		// 	p.httpServer.SourceReadyCallBack, nacos.WithDynamicConfigOption(func(onNacosArgs func(*bootstrap.NacosSourceArgs)) {
		// 		if p.addOnRegArgs != nil {
		// 			p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
		// 				onNacosArgs(args.NacosSource)
		// 			})
		// 		}
		// 	})); err != nil {
		// 	return
		// }
		p.httpServer.HandleFunc(nacos.HttpPath, httpHandle)
		p.httpServer.SourceRegistry(nacos.SourceName)
		if srcArgs.WaitTime > 0 {
			p.httpServer.SourceReadyLater(nacos.SourceName, time.Duration(srcArgs.WaitTime))
		}
		clusterCache = clusterCache || p.regArgs.EurekaSource.LabelPatch
	}

	if srcArgs := p.regArgs.K8SSource; srcArgs.Enabled {
		if kubeSrc, err = p.createKubeSource(kubeResources); err != nil {
			log.Errorf("create k8s source met err %v", err)
			return
		}

		p.httpServer.SourceRegistry(k8s.K8S)
		p.httpServer.SourceReadyLater(k8s.K8S, time.Duration(srcArgs.WaitTime)) // init-done-notify not impl. leave it to do.
	}

	if clusterCache {
		p.initMulticluster()
	}

	if meshConfigFileSrc != nil {
		csrc = append(csrc, meshConfigFileSrc)
	}
	if kubeSrc != nil {
		csrc = append(csrc, kubeSrc)
	}
	if extraKubeSrc != nil {
		csrc = append(csrc, extraKubeSrc)
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

func (p *Processing) getResourceKubeInterfaces() (k kube.Interfaces, err error) {
	if p.regArgs.K8S.ApiServerUrl != "" {
		config, err := clientcmd.BuildConfigFromFlags(p.regArgs.K8S.ApiServerUrl, "")
		if err != nil {
			return nil, err
		}
		k = kube.NewInterfaces(config)
	} else if p.regArgs.K8S.KubeRestConfig != nil {
		k = kube.NewInterfaces(p.regArgs.K8S.KubeRestConfig)
	} else {
		k, err = newInterfaces(p.regArgs.K8S.KubeConfig)
	}
	return
}

func (p *Processing) getDeployKubeClient() (k kube.Interfaces, err error) {
	if p.regArgs.K8S.ApiServerUrlForDeploy != "" {
		config, err := clientcmd.BuildConfigFromFlags(p.regArgs.K8S.ApiServerUrlForDeploy, "")
		if err != nil {
			return nil, err
		}
		k = kube.NewInterfaces(config)
	} else if p.regArgs.K8S.KubeRestConfig != nil {
		k = kube.NewInterfaces(p.regArgs.K8S.KubeRestConfig)
	} else {
		k, err = newInterfaces(p.regArgs.K8S.KubeConfig)
	}
	return
}

func (p *Processing) createFileKubeSource(schemas collection.Schemas, filePath string, watchFiles bool) (
	src event.Source, err error) {
	return fsNew(filePath, schemas, watchFiles)
}

func (p *Processing) createKubeSource(schemas collection.Schemas) (
	src event.Source, err error) {
	o := apiserver.Options{
		Client:            p.k,
		WatchedNamespaces: p.regArgs.K8SSource.WatchedNamespaces,
		ResyncPeriod:      time.Duration(p.regArgs.ResyncPeriod),
		Schemas:           schemas,
	}
	s := apiserver.New(o)
	src = s

	return
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
