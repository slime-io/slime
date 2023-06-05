//go:build generic

package server

import (
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"
	"istio.io/libistio/galley/pkg/config/processor/transforms"
	"istio.io/libistio/galley/pkg/config/util/kuberesource"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/schema"
	"istio.io/libistio/pkg/config/schema/collection"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/mcpoverxds"
	"slime.io/slime/modules/meshregistry/pkg/source/generic"
	"slime.io/slime/modules/meshregistry/pkg/source/generic/eureka"
	"slime.io/slime/modules/meshregistry/pkg/source/generic/nacos"
	"slime.io/slime/modules/meshregistry/pkg/source/k8s"
	"slime.io/slime/modules/meshregistry/pkg/source/zookeeper"
)

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
		if eurekaSrc, httpHandle, err = eureka.Source(
			p.regArgs.EurekaSource,
			srcArgs.NsHost,
			srcArgs.K8sDomainSuffix,
			time.Duration(p.regArgs.RegistryStartDelay),
			p.httpServer.SourceReadyCallBack,
			generic.WithDynamicConfigOption[*eureka.Instance, *eureka.Application](func(onArgs func(*bootstrap.SourceArgs)) {
				if p.addOnRegArgs != nil {
					p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
						onArgs(&args.EurekaSource.SourceArgs)
					})
				}
			})); err != nil {
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
		if nacosSrc, httpHandle, err = nacos.Source(
			p.regArgs.NacosSource,
			srcArgs.NsHost,
			srcArgs.K8sDomainSuffix,
			time.Duration(p.regArgs.RegistryStartDelay),
			p.httpServer.SourceReadyCallBack,
			generic.WithDynamicConfigOption[*nacos.Instance, *nacos.Application](func(onArgs func(*bootstrap.SourceArgs)) {
				if p.addOnRegArgs != nil {
					p.addOnRegArgs(func(args *bootstrap.RegistryArgs) {
						onArgs(&args.NacosSource.SourceArgs)
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
