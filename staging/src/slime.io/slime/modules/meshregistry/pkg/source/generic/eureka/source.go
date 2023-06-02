package eureka

import (
	"net/http"
	"time"

	"istio.io/libistio/pkg/config/event"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source/generic"
)

func Source(
	args *bootstrap.EurekaSourceArgs,
	nsHost bool,
	k8sDomainSuffix bool,
	delay time.Duration,
	readyCallback func(string),
	options ...generic.Option[*Instance, *Application],
) (event.Source, func(http.ResponseWriter, *http.Request), error) {
	servers := args.Servers
	if len(servers) == 0 {
		servers = []bootstrap.EurekaServer{args.EurekaServer}
	}
	client := Clients(servers)
	s, err := generic.NewSource[*Instance, *Application](&args.SourceArgs,
		"eureka", nsHost, k8sDomainSuffix, delay, readyCallback, client, options...)
	if err != nil {
		return nil, nil, err
	}
	return s, s.CacheJson, nil
}
