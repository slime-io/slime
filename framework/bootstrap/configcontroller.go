package bootstrap

import (
	"fmt"
	"net/url"
	"slime.io/slime/framework/bootstrap/viewstore"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap/adsc"
	"slime.io/slime/framework/bootstrap/collections"
	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/serviceregistry/kube"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
	"slime.io/slime/framework/bootstrap/serviceregistry/serviceentry"

	meshconfig "istio.io/api/mesh/v1alpha1"
	mcpresource "istio.io/istio-mcp/pkg/config/schema/resource"
	mcpc "istio.io/istio-mcp/pkg/mcp/client"
	xdsc "istio.io/istio-mcp/pkg/mcp/xds/client"
	mcpmodel "istio.io/istio-mcp/pkg/model"
)

const (
	KubernetesConfigSourcePrefix = "k8s://"
	McpOverXdsConfigSourcePrefix = "xds://"
)

type ConfigController interface {
	viewstore.ViewerStore
	RegisterEventHandler(kind resource.GroupVersionKind, handler func(resource.Config, resource.Config, Event))
	Run(stop <-chan struct{})
}

type configController struct {
	viewerStore        viewstore.ViewerStore
	monitorControllers []*monitorController
	stop               chan<- struct{}
}

func NewConfigController(config *bootconfig.Config, cfg *rest.Config) (ConfigController, error) {
	var cc *configController
	stopCh := make(chan struct{})
	var mcs []*monitorController

	configRevision := config.Global.ConfigRev
	xdsSourceEnableIncPush := config.Global.Misc["xdsSourceEnableIncPush"] == "true"

	// init all monitorControllers
	imc, err := initIstioMonitorController()
	if err != nil {
		return nil, err
	} else {
		mcs = append(mcs, imc)
	}

	if len(config.Global.ConfigSources) > 0 {
		for _, configSource := range config.Global.ConfigSources {
			if strings.HasPrefix(configSource.Address, McpOverXdsConfigSourcePrefix) {
				if mc, err := initXdsMonitorController(configSource); err != nil {
					return nil, err
				} else {
					mcs = append(mcs, mc)
				}
				continue
			}
			if strings.HasPrefix(configSource.Address, KubernetesConfigSourcePrefix) {
				if mc, err := initK8sMonitorController(configSource); err != nil {
					return nil, err
				} else {
					mcs = append(mcs, mc)
				}
				continue
			}
		}
	} else {
		// init k8s for default
		if mc, err := initK8sMonitorController(nil); err != nil {
			return nil, err
		} else {
			mcs = append(mcs, mc)
		}
	}

	vs, err := makeViewerStore(mcs)
	if err != nil {
		return nil, err
	}

	seLabelSelectorKeys := config.Global.Misc["seLabelSelectorKeys"]

	cc = &configController{
		viewerStore:        vs,
		monitorControllers: mcs,
		stop:               stopCh,
	}

	// use cc to register handler for istio resources
	// TODO divide serviceEntry handler to config handler and service handler, like istio
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

	go cc.Run(stopCh)

	for _, mc := range mcs {
		switch mc.kind {
		case Kubernetes:
			if err = startK8sMonitorController(mc, cfg, stopCh); err != nil {
				return nil, err
			}
		case McpOverXds:
			if err = startXdsMonitorController(mc, configRevision, xdsSourceEnableIncPush); err != nil {
				return nil, err
			}
		}
	}

	log.Infof("start ConfigController successfully")

	return cc, nil
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
	// stop k8s informers
	c.stop <- struct{}{}
}

func initIstioMonitorController() (*monitorController, error) {
	ics := makeConfigStore(collections.Istio)
	mc := newMonitorController(ics)
	mc.kind = Istio
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

func startXdsMonitorController(mc *monitorController, configRevision string, xdsSourceEnableIncPush bool) error {

	srcAddress, err := url.Parse(mc.configSource.Address)
	if err != nil {
		return fmt.Errorf("invalid xds config source %s: %v", mc.configSource.Address, err)
	}

	initReqs := adsc.ConfigInitialRequests()
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

	if err = xdsMCP.Run(); err != nil {
		return fmt.Errorf("failed to init client for xds config source %s, err: %+v", mc.configSource.Address, err)
	}

	// check xds cache synced or not
	log.Infof("syncing data of xds config source [%s]", mc.configSource.Address)
	for {
		if xdsMCP.HasSynced() {
			break
		}
		log.Debugf("waiting for syncing data of xds config source [%s]...", mc.configSource.Address)
		time.Sleep(1 * time.Second)
	}

	log.Infof("init xds config source [%s] successfully", mc.configSource.Address)
	return nil
}