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
	"istio.io/libistio/galley/pkg/config/source/kube"
	"istio.io/libistio/galley/pkg/config/source/kube/apiserver"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/mcp/snapshot"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	slimebootstrap "slime.io/slime/framework/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/mcpoverxds"
	"slime.io/slime/modules/meshregistry/pkg/multicluster"
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
