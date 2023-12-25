package source

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"

	"slime.io/slime/modules/meshregistry/pkg/util"
)

var ProtocolHTTP = "HTTP"

type ServiceEntryMergePortMocker struct {
	mergeSvcPorts, mergeInstPorts bool

	resourceName, resourceNs, host string
	resourceLabels                 map[string]string

	dispatcher func(meta resource.Metadata, item *networking.ServiceEntry)

	notifyCh   chan struct{}
	portsCache map[uint32]*networking.ServicePort
	mut        sync.RWMutex
}

func NewServiceEntryMergePortMocker(
	resourceName, resourceNs, serviceHost string, mergeInstPorts, mergeSvcPorts bool,
	resourceLabels map[string]string,
) *ServiceEntryMergePortMocker {
	return &ServiceEntryMergePortMocker{
		resourceName:   resourceName,
		resourceNs:     resourceNs,
		host:           serviceHost,
		mergeInstPorts: mergeInstPorts,
		mergeSvcPorts:  mergeSvcPorts,
		resourceLabels: resourceLabels,

		notifyCh:   make(chan struct{}, 1),
		portsCache: map[uint32]*networking.ServicePort{},
	}
}

// SetDispatcher should be called before `Run`
func (m *ServiceEntryMergePortMocker) SetDispatcher(dispatcher func(meta resource.Metadata, item *networking.ServiceEntry)) {
	m.dispatcher = dispatcher
}

func (m *ServiceEntryMergePortMocker) Refresh() {
	se := &networking.ServiceEntry{
		Hosts:      []string{m.host},
		Ports:      make([]*networking.ServicePort, 0),
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
	if !e.Source.Equal(collections.ServiceEntry) ||
		e.Resource.Metadata.FullName.Name == resource.LocalName(m.resourceName) {
		return
	}

	se := e.Resource.Message.(*networking.ServiceEntry)
	var newPorts []uint32
	m.mut.Lock()
	if m.mergeSvcPorts {
		for _, p := range se.Ports {
			if _, ok := m.portsCache[p.Number]; !ok {
				m.portsCache[p.Number] = p
				newPorts = append(newPorts, p.Number)
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
						port := &networking.ServicePort{
							Number:   portNum,
							Protocol: svcPort.Protocol,
							Name:     fmt.Sprintf("%s-%d", portName, portNum),
						}
						m.portsCache[portNum] = port
						newPorts = append(newPorts, portNum)
						break
					}
				}
			}
		}
	}
	m.mut.Unlock()

	if len(newPorts) > 0 {
		log.Infof("ServiceEntryMergePortMocker: serviceentry %v brings new ports %+v",
			e.Resource.Metadata.FullName, newPorts)
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
		Source: collections.ServiceEntry,
		Resource: &resource.Instance{
			Metadata: meta,
			Message:  se,
		},
	}
}

// ApplyServicePortToEndpoints add svcPort->instPort mappings for those extra svc ports which are mainly from
// MERGE-INSTANCE-PORT-TO-SVC-PORTS.
// For example: we have two ep 1.1.1.1:8080 and 2.2.2.2:8081. After aggregating the svc ports we get http-8080: 8080
// and http-8081: 8081, each instance has one of the corresponding ports.
// After this function takes effect, we get: 1.1.1.1 http-8080: 8080 http-8081: 8080 and 2.2.2.2 http-8080: 8081
// http-8081: 8081
func ApplyServicePortToEndpoints(se *networking.ServiceEntry) {
	if len(se.Ports) == 0 || len(se.Endpoints) == 0 {
		return
	}

	for _, ep := range se.Endpoints {
		if len(ep.Ports) == 0 {
			continue
		}

		var defaultInstPort uint32
		for _, v := range ep.Ports {
			defaultInstPort = v
			break
		}

		for _, svcPort := range se.Ports {
			if _, exist := ep.Ports[svcPort.Name]; !exist {
				ep.Ports[svcPort.Name] = defaultInstPort
			}
		}
	}
}

func PortName(protocol string, num uint32) string {
	return fmt.Sprintf("%s-%d", strings.ToLower(protocol), num)
}

func RectifyServiceEntry(se *networking.ServiceEntry) {
	for _, strs := range [][]string{se.Addresses, se.ExportTo, se.Hosts, se.SubjectAltNames} {
		sort.SliceStable(strs, func(i, j int) bool { return strs[i] < strs[j] })
	}
	sort.SliceStable(se.Endpoints, func(i, j int) bool {
		return se.Endpoints[i].Address < se.Endpoints[j].Address
	})
	sort.SliceStable(se.Ports, func(i, j int) bool {
		return se.Ports[i].Number < se.Ports[j].Number
	})
}
