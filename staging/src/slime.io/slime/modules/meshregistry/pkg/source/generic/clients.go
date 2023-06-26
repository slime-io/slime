package generic

import (
	"fmt"
	"strings"
)

type Clients[I Instance[I], APP Application[I, APP]] []Client[I, APP]

func NewClient[I Instance[I], APP Application[I, APP]](clis ...Client[I, APP]) Clients[I, APP] {
	return clis
}

func (clis Clients[I, APP]) Applications() ([]APP, error) {
	if len(clis) == 1 {
		return clis[0].Applications()
	}
	cache := make(map[string][]I)
	for _, cli := range clis {
		apps, err := cli.Applications()
		if err != nil {
			// We aggregate services with the same name from different server clusters,
			// and if one cluster fails, we return an error but not partial data
			return nil, fmt.Errorf("fetch apps from server %s failed: %v", cli.ServerInfo(), err)
		}
		for _, app := range apps {
			cache[app.GetDomain()] = append([]I(cache[app.GetDomain()]), app.GetInstances()...)
		}
	}
	ret := make([]APP, 0, len(cache))
	for dom, hosts := range cache {
		ir := new(APP)
		ret = append(ret, (*ir).New(dom, hosts))
	}
	return ret, nil
}

func (clis Clients[I, APP]) ServerInfo() string {
	if len(clis) == 1 {
		return clis[0].ServerInfo()
	}
	var infos []string
	for idx, cli := range clis {
		infos = append(infos, fmt.Sprintf("%d:%s", idx, cli.ServerInfo()))
	}
	return strings.Join(infos, ", ")
}
