package nacos

import (
	"net/http"
	"os"
	"strings"
	"time"

	"istio.io/libistio/pkg/config/event"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source/generic"
)

var clientHeadersEnv = "NACOS_CLIENT_HEADERS"

func Source(
	args *bootstrap.NacosSourceArgs,
	nsHost bool,
	k8sDomainSuffix bool,
	delay time.Duration,
	readyCallback func(string),
	options ...generic.Option[*Instance, *Application],
) (event.Source, func(http.ResponseWriter, *http.Request), error) {
	headers := make(map[string]string)
	if nacosHeaders := os.Getenv(clientHeadersEnv); nacosHeaders != "" {
		for _, header := range strings.Split(nacosHeaders, ",") {
			items := strings.SplitN(header, "=", 2)
			if len(items) == 2 {
				headers[items[0]] = items[1]
			}
		}
	}
	servers := args.Servers
	if len(servers) == 0 {
		servers = []bootstrap.NacosServer{args.NacosServer}
	}
	var clis []generic.Client[*Instance, *Application]
	for _, cli := range clients(servers, args.MetaKeyNamespace, args.MetaKeyGroup, headers) {
		clis = append(clis, cli)
	}
	client := generic.NewClient[*Instance, *Application](clis...)
	s, err := generic.NewSource[*Instance, *Application](&args.SourceArgs,
		"nacos", nsHost, k8sDomainSuffix, delay, readyCallback, client, options...)
	if err != nil {
		return nil, nil, err
	}
	return s, s.CacheJson, nil
}
