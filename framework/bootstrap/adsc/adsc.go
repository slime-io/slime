package adsc

import (
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"

	"slime.io/slime/framework/bootstrap/collections"
)

func ConfigInitialRequests() []*discovery.DiscoveryRequest {
	out := make([]*discovery.DiscoveryRequest, 0, len(collections.Pilot.All()))
	for _, sch := range collections.Pilot.All() {
		out = append(out, &discovery.DiscoveryRequest{
			TypeUrl: sch.String(),
		})
	}
	return out
}
