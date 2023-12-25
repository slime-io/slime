package zookeeper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	networkingapi "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"

	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/source"
)

const (
	nonNsSpecSidecarAnno = "sidecar.config.istio.io/nonNsSpec"
)

type DubboCallModel struct {
	Application     string              // dubbo service a
	ProvideServices map[string]struct{} // a provides services
	ConsumeServices map[string]struct{} // services dependent on a
}

// Equals does not distinguish between nil and empty (map)
func (m DubboCallModel) Equals(o DubboCallModel) bool {
	if m.Application != o.Application {
		return false
	}

	if len(m.ProvideServices) != len(o.ProvideServices) {
		return false
	}
	for k := range m.ProvideServices {
		if _, ok := o.ProvideServices[k]; !ok {
			return false
		}
	}

	if len(m.ConsumeServices) != len(o.ConsumeServices) {
		return false
	}
	for k := range m.ConsumeServices {
		if _, ok := o.ConsumeServices[k]; !ok {
			return false
		}
	}

	return true
}

func (m DubboCallModel) Reset() {
}

func (m DubboCallModel) String() string {
	return m.Application
}

func (m DubboCallModel) ProtoMessage() {
}

func (m DubboCallModel) Provide(interfaceName string) bool {
	_, ok := m.ProvideServices[interfaceName]
	return ok
}

func (m DubboCallModel) Consume(interfaceName string) bool {
	_, ok := m.ConsumeServices[interfaceName]
	return ok
}

func (s *Source) serviceEntryHandlerRefreshSidecar(e event.Event) {
	if !e.Source.Equal(collections.ServiceEntry) {
		return
	}

	var preCallModel, callModel map[string]DubboCallModel
	if att := e.Resource.Attachments[AttachmentDubboCallModel]; att != nil {
		callModel = att.(map[string]DubboCallModel)
	}
	s.mut.Lock()
	preCallModel, s.seDubboCallModels[e.Resource.Metadata.FullName] = s.seDubboCallModels[e.Resource.Metadata.FullName], callModel
	changedApps := calcChangedApps(preCallModel, callModel)
	if len(changedApps) > 0 {
		if s.changedApps == nil {
			s.changedApps = map[string]struct{}{}
		}
		for _, app := range changedApps {
			s.changedApps[app] = struct{}{}
		}
	}
	s.mut.Unlock()

	if len(changedApps) > 0 {
		select {
		case s.refreshSidecarNotifyCh <- struct{}{}:
		default:
		}
	}
}

func (s *Source) refreshSidecar(init bool) {
	var changedApps map[string]struct{}
	s.mut.Lock()
	changedApps, s.changedApps = s.changedApps, nil
	s.mut.Unlock()

	if !init && len(changedApps) == 0 { // in init case, ... all
		log.Debugf("refreshSidecar no changed apps")
		return
	}

	log.Infof("refreshSidecar for init %v and changed apps %v", init, changedApps)

	// TODO y: optimize, only re-calc items related to these changedApps

	s.mut.RLock()
	seCallModelsCopy := make(map[resource.FullName]map[string]DubboCallModel, len(s.seDubboCallModels))
	for k, v := range s.seDubboCallModels {
		seCallModelsCopy[k] = v
	}
	prevCallModels := s.dubboCallModels
	s.mut.RUnlock()
	mergedCallModels := mergeDubboCallModels(seCallModelsCopy, false, s.args.SelfConsume)

	diff := diffDubboCallModels(prevCallModels, mergedCallModels)
	if len(diff) == 0 {
		log.Debugf("%d app changed, but merged call models no change(size %d)",
			len(changedApps), len(mergedCallModels))
		return
	} else {
		v, err := json.MarshalIndent(diff, "", "  ")
		log.Infof("dubbo call model diff: %s, json marshal err %v", string(v), err)
	}

	filtered := s.filterDubboCallModelDiff(diff)
	if len(diff) == 0 {
		log.Infof("%d apps changed, but filtered merged call models no change(size %d)",
			len(changedApps), len(mergedCallModels))
		return
	} else if filtered {
		v, err := json.MarshalIndent(diff, "", "  ")
		log.Infof("filtered dubbo call model diff: %s, json marshal err %v", string(v), err)
	}

	s.mut.Lock()
	s.dubboCallModels = mergedCallModels
	s.recordAppSidecarUpdateTime(diff)
	s.mut.Unlock()

	diffSidecars, deletedSidecars := convertDubboCallModelConfigToSidecar(s.args.ResourceNs, mergedCallModels, diff, s.args.DubboWorkloadAppLabel)

	sidecarMap := make(map[resource.FullName]SidecarWithMeta, len(diffSidecars))
	for _, sc := range diffSidecars {
		sidecarMap[sc.Meta.FullName] = sc
	}

	var prevSidecarCache map[resource.FullName]SidecarWithMeta

	s.mut.Lock()
	for k, v := range s.sidecarCache {
		if !deletedSidecars[k] {
			_, ok := sidecarMap[k]
			if !ok {
				sidecarMap[k] = v
			}
		}
	}
	prevSidecarCache, s.sidecarCache = s.sidecarCache, sidecarMap
	s.dubboCallModels = mergedCallModels
	s.mut.Unlock()

	var (
		events                  []event.Event
		added, updated, deleted int
	)
	for fn, cur := range sidecarMap {
		prev, ok := prevSidecarCache[fn]
		if !ok {
			events = append(events, buildSidecarEvent(event.Added, cur.Sidecar, cur.Meta))
			monitoring.RecordSidecarCreation(SourceName)
			added++
		} else if !prev.Equals(cur) {
			events = append(events, buildSidecarEvent(event.Updated, cur.Sidecar, cur.Meta))
			monitoring.RecordSidecarUpdate(SourceName)
			updated++
		}
	}

	if !(added == 0 && len(prevSidecarCache) == len(sidecarMap)) {
		for fn, prev := range prevSidecarCache {
			if _, ok := sidecarMap[fn]; !ok {
				events = append(events, buildSidecarEvent(event.Deleted, prev.Sidecar, prev.Meta))
				monitoring.RecordSidecarDeletion(SourceName)
				deleted++
			}
		}
	}

	if len(events) == 0 {
		log.Warnf("%d apps changed, merged call models changed(size %d -> %d), "+
			"but no sidecars changed",
			len(changedApps), len(prevCallModels), len(mergedCallModels))
		return
	}
	log.Infof("%d apps changed, merged call models changed(size %d -> %d), "+
		"sidecars changed %d, add %d update %d delete %d",
		len(changedApps), len(prevCallModels), len(mergedCallModels), len(events),
		added, updated, deleted)

	for _, ev := range events {
		for _, h := range s.handlers {
			h.Handle(ev)
		}
	}
}

func mergeDubboCallModels(seCallModels map[resource.FullName]map[string]DubboCallModel, includeProvider, selfConsume bool) map[string]DubboCallModel {
	ret := make(map[string]DubboCallModel, len(seCallModels))

	for _, curCallModels := range seCallModels {
		for app, callModel := range curCallModels {
			ret[app] = mergeToDubboCallModel(callModel, ret[app], includeProvider, selfConsume)
		}
	}

	return ret
}

const (
	suffixAdd = "\000+"
	suffixDel = "\000-"
)

func valueFromDiff(v string) string {
	if strings.HasSuffix(v, suffixAdd) {
		return v[:len(v)-len(suffixAdd)]
	} else if strings.HasSuffix(v, suffixDel) {
		return v[:len(v)-len(suffixDel)]
	}
	return v
}

func diffDubboCallModels(prev, cur map[string]DubboCallModel) map[string]DubboCallModel {
	ret := map[string]DubboCallModel{}

	for app, mod := range prev {
		if _, ok := cur[app]; !ok {
			ret[app+suffixDel] = mod
		}
	}

	for app, mod := range cur {
		prevMod, ok := prev[app]
		if !ok {
			ret[app+suffixAdd] = mod
			continue
		}

		diff := DubboCallModel{
			Application:     app,
			ConsumeServices: map[string]struct{}{},
		}

		for svc := range prevMod.ConsumeServices {
			if _, ok := mod.ConsumeServices[svc]; !ok {
				diff.ConsumeServices[svc+suffixDel] = struct{}{}
			}
		}

		for svc := range mod.ConsumeServices {
			if _, ok := prevMod.ConsumeServices[svc]; !ok {
				diff.ConsumeServices[svc] = struct{}{}
			}
		}

		if len(diff.ConsumeServices) > 0 {
			ret[app] = diff
		}
	}

	return ret
}

func (s *Source) filterDubboCallModelDiff(diff map[string]DubboCallModel) bool {
	s.mut.Lock()
	defer s.mut.Unlock()

	var filtered bool

	for app, m := range diff {
		updateTime := s.appSidecarUpdateTime[app]
		// trim-del means will not flush deletions
		trimDel := time.Since(updateTime) < time.Duration(s.args.TrimDubboRemoveDepInterval)

		if strings.HasSuffix(app, suffixDel) && trimDel {
			delete(diff, app)
			filtered = true
		} else if !strings.HasSuffix(app, suffixAdd) {
			for svc := range m.ConsumeServices {
				if strings.HasSuffix(svc, suffixDel) && trimDel {
					delete(m.ConsumeServices, svc)
					filtered = true
				}
			}

			if len(m.ConsumeServices) == 0 {
				delete(diff, app)
				filtered = true
			}
		}
	}

	return filtered
}

func (s *Source) recordAppSidecarUpdateTime(diff map[string]DubboCallModel) {
	// caller should hold the lock
	now := time.Now()
	for app := range diff {
		s.appSidecarUpdateTime[valueFromDiff(app)] = now
	}
}

func mergeToDubboCallModel(from DubboCallModel, to DubboCallModel, includeProvider bool, selfConsume bool) DubboCallModel {
	if to.Application == "" {
		to.Application = from.Application
	}
	if to.ConsumeServices == nil {
		to.ConsumeServices = map[string]struct{}{}
	}

	for svc := range from.ConsumeServices {
		to.ConsumeServices[svc] = struct{}{}
	}
	if selfConsume {
		for svc := range from.ProvideServices {
			to.ConsumeServices[svc] = struct{}{}
		}
	}

	if includeProvider {
		if to.ProvideServices == nil {
			to.ProvideServices = map[string]struct{}{}
		}
		for svc := range from.ProvideServices {
			to.ProvideServices[svc] = struct{}{}
		}
	}
	return to
}

func convertDubboCallModel(se *networkingapi.ServiceEntry, inboundEndpoints []*networkingapi.WorkloadEntry) map[string]DubboCallModel {
	dubboModels := make(map[string]DubboCallModel)

	interfaceName := se.Hosts[0]
	interfaceName = strings.TrimSuffix(interfaceName, DubboHostnameSuffix)

	type item struct {
		eps     []*networkingapi.WorkloadEntry
		inbound bool
	}

	for _, it := range []item{
		{eps: se.Endpoints, inbound: false},
		{eps: inboundEndpoints, inbound: true},
	} {
		for _, e := range it.eps {
			app, ok := e.Labels[DubboSvcAppLabel]
			if !ok {
				continue
			}

			appModel, ok := dubboModels[app]
			if !ok {
				appModel = DubboCallModel{
					Application:     app,
					ConsumeServices: map[string]struct{}{},
					ProvideServices: map[string]struct{}{},
				}
				dubboModels[app] = appModel
			}

			var m map[string]struct{}
			if it.inbound {
				m = appModel.ConsumeServices
			} else {
				m = appModel.ProvideServices
			}

			m[interfaceName] = struct{}{}
		}
	}

	return dubboModels
}

func convertDubboCallModelConfigToSidecar(resourceNs string, callModel map[string]DubboCallModel, diff map[string]DubboCallModel, dubboWorkloadAppLabel string) ([]SidecarWithMeta, map[resource.FullName]bool) {
	var (
		ret             []SidecarWithMeta
		deletedSidecars = map[resource.FullName]bool{}
	)

	now := time.Now()
	for app, m := range callModel {
		fullName := resource.FullName{Namespace: resource.Namespace(resourceNs), Name: resource.LocalName(fmt.Sprintf("%s.dubbo.generated", m.Application))}
		if diff != nil {
			if _, ok := diff[app+suffixDel]; ok {
				// handle app-del standalone
				deletedSidecars[fullName] = true
				continue
			}

			if _, ok := diff[app]; !ok {
				if _, ok = diff[app+suffixAdd]; !ok {
					// not changed call model/sidecar
					continue
				}
			}
		}

		hosts := make([]string, 0, len(m.ConsumeServices))
		for svc := range m.ConsumeServices {
			hosts = append(hosts, wildcardNamespace+"/"+svc)
		}

		sort.Strings(hosts)

		scm := SidecarWithMeta{
			Meta: resource.Metadata{
				FullName:   fullName,
				CreateTime: now,
				Version:    resource.Version(now.String()),
				Annotations: map[string]string{
					nonNsSpecSidecarAnno: "true",
				},
				Labels: map[string]string{},
			},
			Sidecar: &networkingapi.Sidecar{
				WorkloadSelector: &networkingapi.WorkloadSelector{
					Labels: map[string]string{dubboWorkloadAppLabel: m.Application},
				},
				Ingress: nil,
				Egress: []*networkingapi.IstioEgressListener{
					{
						Hosts: hosts,
						Port: &networkingapi.Port{
							Protocol: NetworkProtocolDubbo,
						},
					},
				},
				OutboundTrafficPolicy: nil,
			},
		}

		source.FillRevision(scm.Meta)

		ret = append(ret, scm)
	}

	return ret, deletedSidecars
}

func (s *Source) HandleDubboCallModel(w http.ResponseWriter, request *http.Request) {
	app := request.URL.Query().Get("app")

	s.mut.RLock()
	seCallModelsCopy := make(map[resource.FullName]map[string]DubboCallModel, len(s.seDubboCallModels))
	for k, v := range s.seDubboCallModels {
		seCallModelsCopy[k] = v
	}
	s.mut.RUnlock()
	mergedCallModels := mergeDubboCallModels(seCallModelsCopy, true, s.args.SelfConsume)

	if mergedCallModels != nil && app != "" {
		mergedCallModels = map[string]DubboCallModel{
			app: mergedCallModels[app],
		}
	}

	bs, err := yaml.Marshal(mergedCallModels)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "unable to marshal push dubbo call mode config: %v, %v", err, mergedCallModels)
		return
	}
	w.Header().Add("Content-Type", "text/yaml")

	_, _ = w.Write(bs)
}

func (s *Source) HandleSidecarDubboCallModel(w http.ResponseWriter, request *http.Request) {
	app := request.URL.Query().Get("app")

	s.mut.RLock()
	callModels := s.dubboCallModels
	s.mut.RUnlock()

	if callModels != nil && app != "" {
		callModels = map[string]DubboCallModel{
			app: callModels[app],
		}
	}

	bs, err := yaml.Marshal(callModels)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "unable to marshal push dubbo call mode config: %v, %v", err, callModels)
		return
	}
	w.Header().Add("Content-Type", "text/yaml")

	_, _ = w.Write(bs)
}

func (s *Source) markSidecarInitDone() {
	log.Infof("sidecar init done, call initWg.Done")
	s.initWg.Done()
}

func (s *Source) refreshSidecarTask(stop <-chan struct{}) {
	var (
		waitCh      <-chan time.Time
		waitRefresh int
	)

	for {
		select {
		case <-stop:
			return
		case <-waitCh:
			waitCh = nil
			if waitRefresh == 0 {
				continue
			}
		case <-s.refreshSidecarNotifyCh:
			waitRefresh++
			if waitCh != nil {
				continue
			}
		}

		log.Infof("waitRefresh %d, refresh sidecar", waitRefresh)
		waitRefresh = 0
		s.refreshSidecar(false)
		waitCh = time.After(time.Second)
	}
}

func calcChangedApps(pre, cur map[string]DubboCallModel) []string {
	var ret []string

	for app, mo := range pre {
		curMo, ok := cur[app]
		if !ok || !mo.Equals(curMo) {
			ret = append(ret, app)
		}
	}

	for app := range cur {
		if _, ok := pre[app]; !ok {
			ret = append(ret, app)
		}
	}

	return ret
}
