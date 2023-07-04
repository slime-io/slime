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

	filter := []model.Instance{model.HTTP}
	if r.cfg.SupportH2 {
		filter = append(filter, model.HTTP2, model.GRPC, model.GRPCWeb)
	}

	for _, svc := range istioSvcs {
		for _, port := range svc.Ports {

			if !protocolFilter(filter, port.Protocol) {
				continue
			}
			
			p := int32(port.Port)
			r.portProtocolCache.Lock()

			if _, ok := r.portProtocolCache.Data[p]; !ok {
				r.portProtocolCache.Data[p] = make(map[Protocol]uint)
			}
			r.portProtocolCache.Data[p][ListenerProtocolHTTP]++
			log.Debugf("get serviceentry http port %d", p)
			r.portProtocolCache.Unlock()
		}
	}
}

func protocolFilter(arr []model.Instance, val model.Instance) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}
