package controllers

import (
	"context"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
	"slime.io/slime/framework/bootstrap/serviceregistry/serviceentry"
)

func (r *ServicefenceReconciler) registerSeHandler(ctx context.Context) {
	log := log.WithField("function", "StartSeCache")
	seTocache := func(old resource.Config, cfg resource.Config, e bootstrap.Event) {
		switch e {
		case bootstrap.EventAdd:
			istioSvcs, _ := serviceentry.ConvertSvcsAndEps(cfg, "")
			log.Infof("get istioSvc %+v", istioSvcs)
			r.cachePort(istioSvcs)
		default:
		}
	}
	log.Infof("lazyload: register serviceEntry handler")
	r.env.ConfigController.RegisterEventHandler(resource.IstioService, seTocache)
}

func (r *ServicefenceReconciler) cachePort(istioSvcs []*model.Service) {
	for _, svc := range istioSvcs {
		for _, port := range svc.Ports {
			if port.Protocol != model.HTTP {
				continue
			}
			p := int32(port.Port)
			r.portProtocolCache.Lock()

			if _, ok := r.portProtocolCache.Data[p]; !ok {
				r.portProtocolCache.Data[p] = make(map[Protocol]uint)
			}
			// decode port name
			r.portProtocolCache.Data[p][ProtocolHTTP]++
			r.portProtocolCache.Unlock()
		}
	}
}
