package mcpoverxds

import (
	"sync"
	"time"

	"istio.io/istio-mcp/pkg/config/schema/resource"
	mcpmodel "istio.io/istio-mcp/pkg/model"
)

type McpConfigStore struct {
	sync.RWMutex
	snaps               map[string]map[resource.GroupVersionKind]map[string]*mcpmodel.Config
	nsVersions          map[string]string
	gvkResourceVersions map[resource.GroupVersionKind]string
}

func NewConfigStore() *McpConfigStore {
	return &McpConfigStore{
		snaps:               make(map[string]map[resource.GroupVersionKind]map[string]*mcpmodel.Config),
		nsVersions:          make(map[string]string),
		gvkResourceVersions: map[resource.GroupVersionKind]string{},
	}
}

func strNow() string {
	return time.Now().String()
}

func (s *McpConfigStore) Update(ns string, gvk resource.GroupVersionKind, name string, config *mcpmodel.Config) (revChanged bool) {
	var prev *mcpmodel.Config

	s.Lock()
	defer s.Unlock()

	defer func() {
		if prev == nil || config == nil {
			return
		}

		revChanged = mcpmodel.ConfigIstioRev(prev) != mcpmodel.ConfigIstioRev(config)
	}()

	tnow := strNow()
	if config != nil {
		if config.ResourceVersion == "" { // generate/update resource version for internal(self-gen) configs
			config.ResourceVersion = tnow
			mcpmodel.UpdateAnnotationResourceVersion(config)
		}

		if config.ResourceVersion > s.gvkResourceVersions[gvk] { // should always be true
			s.gvkResourceVersions[gvk] = config.ResourceVersion
		}
	}

	nsConfigs := s.snaps[ns]
	if nsConfigs == nil {
		if config == nil {
			return
		}
		nsConfigs = map[resource.GroupVersionKind]map[string]*mcpmodel.Config{}
		s.snaps[ns] = nsConfigs
	}

	gvkConfigs := nsConfigs[gvk]
	if gvkConfigs == nil {
		if config == nil {
			return
		}
		gvkConfigs = map[string]*mcpmodel.Config{}
		nsConfigs[gvk] = gvkConfigs
	}

	prev = gvkConfigs[name]
	if config == nil {
		delete(gvkConfigs, name)
	} else {
		gvkConfigs[name] = config
	}

	s.nsVersions[ns] = tnow

	return
}

func (s *McpConfigStore) Version(ns string) string {
	s.RLock()
	defer s.RUnlock()
	ret := ""
	if ns == resource.AllNamespace {
		for _, ver := range s.nsVersions {
			if ver > ret {
				ret = ver
			}
		}
	} else {
		ver := s.nsVersions[ns]
		return ver
	}
	return ret
}

func (s *McpConfigStore) Get(gvk resource.GroupVersionKind, namespace, name string) (*mcpmodel.Config, error) {
	s.RLock()
	defer s.RUnlock()

	cfg := s.snaps[namespace][gvk][name]
	if cfg == nil {
		return nil, nil
	}

	cfgCopy := *cfg
	return &cfgCopy, nil
}

func (s *McpConfigStore) List(gvk resource.GroupVersionKind, namespace, ver string) ([]mcpmodel.Config, string, error) {
	var (
		ret    []mcpmodel.Config
		retVer string
	)

	s.RLock()
	defer s.RUnlock()

	snaps := s.snaps
	if namespace != resource.AllNamespace {
		snaps = map[string]map[resource.GroupVersionKind]map[string]*mcpmodel.Config{
			namespace: snaps[namespace],
		}
	}

	for _, nsConfigs := range snaps {
		if gvk != resource.AllGvk {
			nsConfigs = map[resource.GroupVersionKind]map[string]*mcpmodel.Config{
				gvk: nsConfigs[gvk],
			}
		}

		for _, gvkConfigs := range nsConfigs {
			for _, conf := range gvkConfigs {
				if ver == "" || conf.ResourceVersion > ver {
					if conf.ResourceVersion > retVer {
						retVer = conf.ResourceVersion
					}
					ret = append(ret, *conf)
				}
			}
		}
	}

	for idx := range ret {
		ret[idx].ResourceVersion = "" // to explicitly override
	}
	return ret, retVer, nil
}

func (s *McpConfigStore) Print(ns string, gvk resource.GroupVersionKind, name string) map[string][]*mcpmodel.Config {
	s.Lock() // Use write lock as this op will be pretty slow.
	defer s.Unlock()

	res := make(map[resource.GroupVersionKind][]*mcpmodel.Config)

	snaps := s.snaps
	if ns != resource.AllNamespace {
		snaps = map[string]map[resource.GroupVersionKind]map[string]*mcpmodel.Config{
			ns: snaps[ns],
		}
	}

	for _, nsConfigs := range snaps {
		if gvk != resource.AllGvk {
			nsConfigs = map[resource.GroupVersionKind]map[string]*mcpmodel.Config{
				gvk: nsConfigs[gvk],
			}
		}

		for gvk, gvkConfigs := range nsConfigs {
			for resName, conf := range gvkConfigs {
				if name != "" && resName != name {
					continue
				} else {
					res[gvk] = append(res[gvk], conf)
				}
			}
		}
	}

	res1 := make(map[string][]*mcpmodel.Config, len(res))
	for gvk, configs := range res {
		res1[gvk.String()] = configs
	}

	return res1
}

func (s *McpConfigStore) Snapshot(ns string) mcpmodel.ConfigSnapshot {
	return nil
}

func (s *McpConfigStore) VersionSnapshot(version, ns string) mcpmodel.ConfigSnapshot {
	// TODO
	return nil
}

func (s *McpConfigStore) dumpGvkResourceVersions() map[resource.GroupVersionKind]string {
	s.RLock()
	defer s.RUnlock()
	ret := make(map[resource.GroupVersionKind]string, len(s.gvkResourceVersions))
	for k, v := range s.gvkResourceVersions {
		ret[k] = v
	}
	return ret
}

func (s *McpConfigStore) ClearZombie(gvkMinVers map[resource.GroupVersionKind]string) map[resource.GroupVersionKind]int {
	cnt := map[resource.GroupVersionKind]int{}
	s.Lock()
	defer s.Unlock()

	for gvk, minVer := range gvkMinVers {
		if minVer == "" {
			continue
		}
		for _, nsConfigs := range s.snaps {
			gvkConfigs := nsConfigs[gvk]
			for name, cfg := range gvkConfigs {
				if cfg.Spec == nil && cfg.ResourceVersion <= minVer {
					delete(gvkConfigs, name)
					cnt[gvk]++
				}
			}
		}
	}

	return cnt
}
