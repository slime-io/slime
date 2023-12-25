package bootstrap

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	log "github.com/sirupsen/logrus"
	meshconfig "istio.io/api/mesh/v1alpha1"
	mcpresource "istio.io/istio-mcp/pkg/config/schema/resource"
	mcpc "istio.io/istio-mcp/pkg/mcp/client"
	xdsc "istio.io/istio-mcp/pkg/mcp/xds/client"
	mcpmodel "istio.io/istio-mcp/pkg/model"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap/adsc"
	"slime.io/slime/framework/bootstrap/collections"
	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/serviceregistry/kube"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
	"slime.io/slime/framework/bootstrap/serviceregistry/serviceentry"
	"slime.io/slime/framework/bootstrap/viewstore"
)

const (
	KubernetesConfigSourcePrefix = "k8s://"
	McpOverXdsConfigSourcePrefix = "xds://"
)

type InitReady interface {
	InitReady() bool
}

type ConfigController interface {
	viewstore.ViewerStore
	RegisterEventHandler(kind resource.GroupVersionKind, handler func(resource.Config, resource.Config, Event))
	Run(stop <-chan struct{})
	InitReady
}

type configController struct {
	viewerStore        viewstore.ViewerStore
	monitorControllers []*monitorController
	stop               <-chan struct{}
}

func RunIstioController(c ConfigController, config *bootconfig.Config) (ConfigController, error) {
	cc, ok := c.(*configController)
	if !ok {
		return c, fmt.Errorf("convert to *configController error")
	}
	xdsSourceEnableIncPush := config.Global.Misc["xdsSourceEnableIncPush"] == "true"
	stop := cc.stop

	go cc.Run(stop)
	for _, mc := range cc.monitorControllers {
		switch mc.kind {
		case McpOverXds:
			if err := startXdsMonitorController(mc, "", xdsSourceEnableIncPush, stop); err != nil {
				return nil, err
			}
		}
	}
	log.Infof("start IstioConfigController successfully")
	return cc, nil
}

func NewConfigController(configSources []*bootconfig.ConfigSource, stop <-chan struct{}) (*configController, error) {
	var mcs []*monitorController

	// init all monitorControllers
	imc, err := initIstioMonitorController()
	if err != nil {
		return nil, err
	} else {
		mcs = append(mcs, imc)
	}
	for _, configSource := range configSources {
		if strings.HasPrefix(configSource.Address, McpOverXdsConfigSourcePrefix) {
			if mc, err := initXdsMonitorController(configSource); err != nil {
				return nil, err
			} else {
				mcs = append(mcs, mc)
			}
		} else if strings.HasPrefix(configSource.Address, KubernetesConfigSourcePrefix) {
			if mc, err := initK8sMonitorController(configSource); err != nil {
				return nil, err
			} else {
				mcs = append(mcs, mc)
			}
		} else {
			log.Warnf("configsource address %s is not supported", configSource.Address)
		}
	}

	if len(mcs) == 1 {
		return nil, fmt.Errorf("no valid address in configSource address")
	}

	vs, err := makeViewerStore(mcs)
	if err != nil {
		return nil, err
	}

	cc := &configController{
		viewerStore:        vs,
		monitorControllers: mcs,
		stop:               stop,
	}
	return cc, nil
}

func RunController(c ConfigController, config *bootconfig.Config, cfg *rest.Config) (ConfigController, error) {
	cc, ok := c.(*configController)
	if !ok {
		return c, fmt.Errorf("convert to *configController error")
	}

	var err error
	imc := cc.monitorControllers[0]
	vs := cc.viewerStore
	stop := cc.stop
	mcs := cc.monitorControllers
	seLabelSelectorKeys := config.Global.Misc["seLabelSelectorKeys"]
	configRevision := config.Global.ConfigRev
	xdsSourceEnableIncPush := config.Global.Misc["xdsSourceEnableIncPush"] == "true"

	// use cc to register handler for istio resources
	if config.Global.EnableConvertSeToIstioRes {

		seToIstioResHandler := func(old resource.Config, cfg resource.Config, e Event) {
			switch e {
			case EventAdd:
				istioSvcs, istioEps := serviceentry.ConvertSvcsAndEps(cfg, seLabelSelectorKeys)
				for _, istioSvc := range istioSvcs {
					_, err = imc.Create(istioSvc.ConvertConfig())
					if err != nil {
						log.Errorf("[seToIstioResHandler] [EventAdd] for IstioService error: %+v", err)
						return
					}
				}
				for _, istioEp := range istioEps {
					if _, err = imc.Create(istioEp.ConvertConfig()); err != nil {
						log.Errorf("[seToIstioResHandler] [EventAdd] for IstioEndpoint error: %+v", err)
						return
					}
				}
			case EventUpdate:
				oldIstioSvcs, oldIstioEps := serviceentry.ConvertSvcsAndEps(old, seLabelSelectorKeys)
				for _, oldIstioSvc := range oldIstioSvcs {
					c := oldIstioSvc.ConvertConfig()
					err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace)
					if err != nil {
						log.Errorf("[seToIstioResHandler] [EventUpdate] delete old IstioService error: %+v", err)
						return
					}
				}
				for _, oldIstioEp := range oldIstioEps {
					c := oldIstioEp.ConvertConfig()
					err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace)
					if err != nil {
						log.Errorf("[seToIstioResHandler] [EventUpdate] delete old IstioEndpoint error: %+v", err)
						return
					}
				}
				newIstioSvcs, newIstioEps := serviceentry.ConvertSvcsAndEps(cfg, seLabelSelectorKeys)
				for _, newIstioSvc := range newIstioSvcs {
					_, err = imc.Create(newIstioSvc.ConvertConfig())
					if err != nil {
						log.Errorf("[seToIstioResHandler] [EventUpdate] create new IstioService error: %+v", err)
						return
					}
				}
				for _, newIstioEp := range newIstioEps {
					_, err = imc.Create(newIstioEp.ConvertConfig())
					if err != nil {
						log.Errorf("[seToIstioResHandler] [EventUpdate] create new IstioEndpoint error: %+v", err)
						return
					}
				}
			case EventDelete:
				istioSvcs, istioEps := serviceentry.ConvertSvcsAndEps(cfg, seLabelSelectorKeys)
				for _, istioSvc := range istioSvcs {
					c := istioSvc.ConvertConfig()
					err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace)
					if err != nil {
						log.Errorf("[seToIstioResHandler] [EventDelete] for IstioService error: %+v", err)
						return
					}
				}
				for _, istioEp := range istioEps {
					c := istioEp.ConvertConfig()
					err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace)
					if err != nil {
						log.Errorf("[seToIstioResHandler] [EventDelete] for IstioEndpoint error: %+v", err)
						return
					}
				}
			}
		}

		svcToIstioResHandler := func(old resource.Config, cfg resource.Config, e Event) {
			switch e {
			case EventAdd:
				service, eps, err := kube.ConvertSvcAndEps(cfg, vs)
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventAdd] ConvertSvcAndEps error: %+v", err)
					return
				}
				_, err = imc.Create(service.ConvertConfig())
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventAdd] for service error: %+v", err)
					return
				}
				for _, ep := range eps {
					if _, err = imc.Create(ep.ConvertConfig()); err != nil {
						log.Errorf("[svcToIstioResHandler] [EventAdd] for endpoint error: %+v", err)
						return
					}
				}
			case EventUpdate:
				oldService, oldEps, err := kube.ConvertSvcAndEps(old, vs)
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventUpdate] ConvertSvcAndEps error: %+v", err)
					return
				}
				c := oldService.ConvertConfig()
				err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace)
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventUpdate] delete old service error: %+v", err)
					return
				}
				for _, oldEp := range oldEps {
					c = oldEp.ConvertConfig()
					if err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace); err != nil {
						log.Errorf("[svcToIstioResHandler] [EventUpdate] delete old endpoint error: %+v", err)
						return
					}
				}

				newService, newEps, err := kube.ConvertSvcAndEps(cfg, vs)
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventUpdate] ConvertSvcAndEps error: %+v", err)
					return
				}
				_, err = imc.Create(newService.ConvertConfig())
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventUpdate] create new service error: %+v", err)
					return
				}
				for _, newEp := range newEps {
					if _, err = imc.Create(newEp.ConvertConfig()); err != nil {
						log.Errorf("[svcToIstioResHandler] [EventUpdate] create new endpoint error: %+v", err)
						return
					}
				}
			case EventDelete:
				service, eps, err := kube.ConvertSvcAndEps(cfg, vs)
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventUpdate] ConvertSvcAndEps error: %+v", err)
					return
				}
				c := service.ConvertConfig()
				err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace)
				if err != nil {
					log.Errorf("[svcToIstioResHandler] [EventDelete] for service error: %+v", err)
					return
				}
				for _, ep := range eps {
					c = ep.ConvertConfig()
					if err = imc.Delete(c.GroupVersionKind, c.Name, c.Namespace); err != nil {
						log.Errorf("[svcToIstioResHandler] [EventDelete] for endpoint error: %+v", err)
						return
					}
				}
			}
		}

		epsToIstioResHandler := func(old resource.Config, cfg resource.Config, e Event) {
			ep := cfg.Spec.(*v1.Endpoints)
			hostname := kube.ServiceHostname(cfg.Name, cfg.Namespace, model.DefaultTrustDomain)

			switch e {
			case EventAdd:
				// handle by svcHandler
			case EventUpdate:
				isCfg := imc.Get(resource.IstioService, string(hostname), cfg.Namespace)
				if isCfg == nil {
					return
				}
				is := isCfg.Spec.(*model.Service)

				oldEp := old.Spec.(*v1.Endpoints)
				oldIeps, err := kube.ConvertIstioEndpoints(oldEp, hostname, old.Name, old.Namespace, vs)
				if err != nil {
					log.Errorf("[epsToIstioResHandler] [EventUpdate] ConvertIstioEndpoints error: %+v", err)
					return
				}
				for _, oldIep := range oldIeps {
					oldIepName := oldIep.ServiceName + "/" + oldIep.ServicePortName + "/" + oldIep.Address + ":" + strconv.Itoa(int(oldIep.EndpointPort))
					if err = imc.Delete(resource.IstioEndpoint, oldIepName, oldIep.Namespace); err != nil {
						log.Errorf("[epsToIstioResHandler] [EventUpdate] delete old IstioEndpoint error: %+v", err)
						return
					}
					// delete related istioService.Endpoints
					for i, ep := range is.Endpoints {
						if ep.Address == oldIep.Address && ep.EndpointPort == oldIep.EndpointPort {
							l := len(is.Endpoints)
							if i != l-1 {
								is.Endpoints[i], is.Endpoints[l-1] = is.Endpoints[l-1], is.Endpoints[i]
							}
							is.Endpoints = is.Endpoints[:l-1]
							break
						}
					}
				}

				newIeps, err := kube.ConvertIstioEndpoints(ep, hostname, cfg.Name, cfg.Namespace, vs)
				if err != nil {
					log.Errorf("[epsToIstioResHandler] [EventUpdate] ConvertIstioEndpoints error: %+v", err)
					return
				}
				for _, newIep := range newIeps {
					if _, err = imc.Create(newIep.ConvertConfig()); err != nil {
						log.Errorf("[epsToIstioResHandler] [EventUpdate] create new IstioEndpoint error: %+v", err)
						return
					}
					// add related istioService.Endpoints
					is.Endpoints = append(is.Endpoints, newIep)
				}

				if _, err := imc.Update(is.ConvertConfig()); err != nil {
					log.Errorf("[epsToIstioResHandler] [EventUpdate] update IstioService error: %+v", err)
					return
				}
			case EventDelete:
				// handle by svcHandler
			}
		}

		cc.RegisterEventHandler(resource.ServiceEntry, seToIstioResHandler)
		cc.RegisterEventHandler(resource.Service, svcToIstioResHandler)
		cc.RegisterEventHandler(resource.Endpoints, epsToIstioResHandler)
	}

	go cc.Run(stop)

	for _, mc := range mcs {
		switch mc.kind {
		case Kubernetes:
			if err = startK8sMonitorController(mc, cfg, stop); err != nil {
				return nil, err
			}
		case McpOverXds:
			if err = startXdsMonitorController(mc, configRevision, xdsSourceEnableIncPush, stop); err != nil {
				return nil, err
			}
		}
	}

	log.Infof("start ConfigController successfully")
	return cc, nil
}

func (c *configController) InitReady() bool {
	for _, mc := range c.monitorControllers {
		if !mc.InitReady() {
			return false
		}
	}
	return true
}

func (c *configController) Schemas() resource.Schemas {
	return c.viewerStore.Schemas()
}

func (c *configController) Get(gvk resource.GroupVersionKind, name, namespace string) *resource.Config {
	return c.viewerStore.Get(gvk, name, namespace)
}

func (c *configController) List(gvk resource.GroupVersionKind, namespace string) ([]resource.Config, error) {
	return c.viewerStore.List(gvk, namespace)
}

func (c *configController) RegisterEventHandler(kind resource.GroupVersionKind, handler func(resource.Config, resource.Config, Event)) {
	for _, mc := range c.monitorControllers {
		if _, exists := mc.Schemas().FindByGroupVersionKind(kind); exists {
			mc.RegisterEventHandler(kind, handler)
			log.Debugf("RegisterEventHandler success for gvk %s", kind.String())
		}
	}
}

func (c *configController) Run(stop <-chan struct{}) {
	for i := range c.monitorControllers {
		go c.monitorControllers[i].Run(stop)
	}
	<-stop
	log.Infof("stop controller run")
}

func initIstioMonitorController() (*monitorController, error) {
	ics := makeConfigStore(collections.Istio)
	mc := newMonitorController(ics)
	mc.kind = Istio
	mc.SetReady()
	return mc, nil
}

func initXdsMonitorController(configSource *bootconfig.ConfigSource) (*monitorController, error) {
	xcs := makeConfigStore(collections.Pilot)
	mc := newMonitorController(xcs)
	mc.kind = McpOverXds
	mc.configSource = configSource
	log.Infof("init xds config source [%s] successfully", configSource.Address)
	return mc, nil
}

func startXdsMonitorController(mc *monitorController, configRevision string, xdsSourceEnableIncPush bool, stop <-chan struct{}) error {
	srcAddress, err := url.Parse(mc.configSource.Address)
	if err != nil {
		return fmt.Errorf("invalid xds config source %s: %v", mc.configSource.Address, err)
	}
	types := srcAddress.Query()["types"]

	initReqs := adsc.ConfigInitialRequests()
	if types != nil {
		var filteredReqs []*discovery.DiscoveryRequest
		for _, req := range initReqs {
			var match bool
			gvk, err := resource.ParseGroupVersionKind(req.TypeUrl)
			if err != nil {
				log.Errorf("parse req.TypeUrl %s error: %+v", err)
				continue
			}
			for _, t := range types {
				if t == req.TypeUrl || strings.EqualFold(gvk.Kind, t) {
					match = true
					break
				}
			}
			if match {
				filteredReqs = append(filteredReqs, req)
			}
		}
		initReqs = filteredReqs
	}

	var initReqTypes []string
	for _, r := range initReqs {
		initReqTypes = append(initReqTypes, r.TypeUrl)
	}

	configHandlerAdapter := &mcpc.ConfigStoreHandlerAdapter{
		List: func(mcpgvk mcpresource.GroupVersionKind, namespace string) ([]mcpmodel.NamespacedName, error) {
			gvk := resource.GroupVersionKind{
				Group:   mcpgvk.Group,
				Version: mcpgvk.Version,
				Kind:    mcpgvk.Kind,
			}
			configs, err := mc.List(gvk, namespace)
			if err != nil {
				return nil, err
			}
			var ret []mcpmodel.NamespacedName
			for _, c := range configs {
				ret = append(ret, mcpmodel.NamespacedName{
					Name:      c.Name,
					Namespace: c.Namespace,
				})
			}
			return ret, nil
		},

		AddOrUpdate: func(mcpcfg mcpmodel.Config) (mcpc.Change, string, string, error) {
			gvk := resource.GroupVersionKind{
				Group:   mcpcfg.GroupVersionKind.Group,
				Version: mcpcfg.GroupVersionKind.Version,
				Kind:    mcpcfg.GroupVersionKind.Kind,
			}

			cfg := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  gvk,
					Name:              mcpcfg.Name,
					Namespace:         mcpcfg.Namespace,
					Domain:            mcpcfg.Domain,
					Labels:            mcpcfg.Labels,
					Annotations:       mcpcfg.Annotations,
					ResourceVersion:   mcpcfg.ResourceVersion,
					CreationTimestamp: mcpcfg.CreationTimestamp,
				},
				Spec: mcpcfg.Spec,
			}

			var (
				err             error
				existed         = mc.Get(gvk, mcpcfg.Name, mcpcfg.Namespace)
				ch              mcpc.Change
				newRev, prevRev string
			)
			if existed == nil {
				newRev, err = mc.Create(cfg)
				ch = mcpc.ChangeAdd
			} else {
				prevRev = existed.ResourceVersion
				newRev, err = mc.Update(cfg)
				if newRev == cfg.ResourceVersion {
					ch = mcpc.ChangeNoUpdate
				} else {
					ch = mcpc.ChangeUpdate
				}
			}

			return ch, prevRev, newRev, err
		},

		Del: func(mcpgvk mcpresource.GroupVersionKind, name, namespace string) error {
			return mc.Delete(resource.GroupVersionKind{
				Group:   mcpgvk.Group,
				Version: mcpgvk.Version,
				Kind:    mcpgvk.Kind,
			}, name, namespace)
		},

		Inc: xdsSourceEnableIncPush,
	}

	// mcpCli handles mcp data
	mcpCli := mcpc.NewAdsc(&mcpc.Config{
		// To reduce transported data if upstream server supports. Especially for custom servers.
		Revision:           configRevision,
		InitReqTypes:       initReqTypes,
		TypeConfigsHandler: configHandlerAdapter.TypeConfigsHandler,
	})
	configHandlerAdapter.A = mcpCli

	// xdsMCP handles xds data
	xdsMCP, err := xdsc.New(&meshconfig.ProxyConfig{
		DiscoveryAddress: srcAddress.Host,
	}, &xdsc.Config{
		Meta: resource.NodeMetadata{
			Generator:     "api",
			IstioRevision: configRevision,
		}.ToStruct(),
		InitialDiscoveryRequests: initReqs,
		DiscoveryHandler:         mcpCli,
		StateNotifier: func(state xdsc.State) {
			if state == xdsc.StateConnected {
				configHandlerAdapter.Reset()
			}
		},
	})

	go func() {
		log.Infof("MCP: connect xds source and wait sync in background")
		err := xdsMCP.Run()
		if err != nil {
			log.Errorf("MCP: failed running %v", err)
			return
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				log.Infof("get stop chan in XdsMonitorController")
				return
			case <-ticker.C:
				if xdsMCP.HasSynced() {
					log.Infof("sync xds config source [%s] successfully", mc.configSource.Address)
					mc.SetReady()
					return
				}
				log.Debugf("waiting for syncing data of xds config source [%s]...", mc.configSource.Address)
			}
		}
	}()

	return nil
}
