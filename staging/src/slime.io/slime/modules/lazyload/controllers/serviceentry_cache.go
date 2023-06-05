package controllers

import (
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/serviceregistry/model"
	"slime.io/slime/framework/bootstrap/serviceregistry/serviceentry"
)

func (r *ServicefenceReconciler) RegisterSeHandler() {

	// only add, not delete
	sePortToCache := func(old resource.Config, cfg resource.Config, e bootstrap.Event) {
		switch e {
		case bootstrap.EventAdd:
			istioSvcs, _ := serviceentry.ConvertSvcsAndEps(cfg, "")
			r.cachePort(istioSvcs)
		case bootstrap.EventUpdate:
			istioSvcs, _ := serviceentry.ConvertSvcsAndEps(cfg, "")
			r.cachePort(istioSvcs)
		default:
		}
	}
	log.Infof("lazyload: register serviceEntry handler")

	r.env.ConfigController.RegisterEventHandler(resource.ServiceEntry, sePortToCache)
}

func (r *ServicefenceReconciler) cachePort(istioSvcs []*model.Service) {
	for _, svc := range istioSvcs {
		for _, port := range svc.Ports {
			if port.Protocol != model.HTTP && port.Protocol != model.GRPC && port.Protocol != model.HTTP2 {
				continue
			}
			p := int32(port.Port)
			r.portProtocolCache.Lock()

			if _, ok := r.portProtocolCache.Data[p]; !ok {
				r.portProtocolCache.Data[p] = make(map[Protocol]uint)
			}
			r.portProtocolCache.Data[p][ProtocolHTTP]++
			log.Debugf("get serviceentry http port %d", p)
			r.portProtocolCache.Unlock()
		}
	}
}
