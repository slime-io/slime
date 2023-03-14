package source

import (
	"fmt"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"
	"slime.io/slime/modules/meshregistry/pkg/util"
	"sort"
	"sync"
	"time"
)

type ServiceEntryMergePortMocker struct {
	mergeSvcPorts, mergeInstPorts bool

	resourceName, resourceNs, host string
	resourceLabels                 map[string]string

	dispatcher func(meta resource.Metadata, item *networking.ServiceEntry)

	notifyCh   chan struct{}
	portsCache map[uint32]*networking.Port
	mut        sync.RWMutex
}

func NewServiceEntryMergePortMocker(
	resourceName, resourceNs, serviceHost string, mergeInstPorts, mergeSvcPorts bool,
	resourceLabels map[string]string) *ServiceEntryMergePortMocker {
	return &ServiceEntryMergePortMocker{
		resourceName:   resourceName,
		resourceNs:     resourceNs,
		host:           serviceHost,
		mergeInstPorts: mergeInstPorts,
		mergeSvcPorts:  mergeSvcPorts,
		resourceLabels: resourceLabels,

		notifyCh:   make(chan struct{}, 1),
		portsCache: map[uint32]*networking.Port{},
	}
}

// SetDispatcher should be called before `Run`
func (m *ServiceEntryMergePortMocker) SetDispatcher(dispatcher func(meta resource.Metadata, item *networking.ServiceEntry)) {
	m.dispatcher = dispatcher
}

func (m *ServiceEntryMergePortMocker) Refresh() {
	se := &networking.ServiceEntry{
		Hosts:      []string{m.host},
		Ports:      make([]*networking.Port, 0),
		Resolution: networking.ServiceEntry_STATIC,
	}

	m.mut.RLock()
	for _, p := range m.portsCache {
		se.Ports = append(se.Ports, p)
	}
	m.mut.RUnlock()
	sort.Slice(se.Ports, func(i, j int) bool {
		return se.Ports[i].Number < se.Ports[j].Number
	})

	if m.dispatcher != nil {
		now := time.Now()
		lbls := map[string]string{}
		for k, v := range m.resourceLabels {
			lbls[k] = v
		}
		meta := resource.Metadata{
			FullName:    resource.FullName{Namespace: resource.Namespace(m.resourceNs), Name: resource.LocalName(m.resourceName)},
			CreateTime:  now,
			Version:     resource.Version(now.String()),
			Labels:      lbls,
			Annotations: map[string]string{},
		}
		m.dispatcher(meta, se)
	}
}

func (m *ServiceEntryMergePortMocker) Start(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case <-m.notifyCh:
			m.Refresh()
		}
	}
}

func (m *ServiceEntryMergePortMocker) Handle(e event.Event) {
	if e.Source != collections.K8SNetworkingIstioIoV1Alpha3Serviceentries || e.Resource.Metadata.FullName.Name == resource.LocalName(m.resourceName) {
		return
	}

	se := e.Resource.Message.(*networking.ServiceEntry)
	var newPort bool
	m.mut.Lock()
	if m.mergeSvcPorts {
		for _, p := range se.Ports {
			if _, ok := m.portsCache[p.Number]; !ok {
				m.portsCache[p.Number] = p
				newPort = true
			}
		}
	}

	if m.mergeInstPorts {
		for _, ep := range se.Endpoints {
			for portName, portNum := range ep.Ports {
				_, ok := m.portsCache[portNum]
				if ok {
					continue
				}

				for _, svcPort := range se.Ports {
					if svcPort.Name == portName {
						port := &networking.Port{
							Number:   portNum,
							Protocol: svcPort.Protocol,
							Name:     fmt.Sprintf("%s-%d", portName, portNum),
						}
						m.portsCache[portNum] = port
						break
					}
				}
			}
		}
	}
	m.mut.Unlock()

	if newPort {
		select {
		case m.notifyCh <- struct{}{}:
		default:
		}
	}
}

func BuildServiceEntryEvent(kind event.Kind, se *networking.ServiceEntry, meta resource.Metadata) event.Event {
	FillRevision(meta)
	util.FillSeLabels(se, meta)
	return event.Event{
		Kind:   kind,
		Source: collections.K8SNetworkingIstioIoV1Alpha3Serviceentries,
		Resource: &resource.Instance{
			Metadata: meta,
			Message:  se,
		},
	}
}
