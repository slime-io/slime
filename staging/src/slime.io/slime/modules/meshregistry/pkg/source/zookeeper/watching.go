package zookeeper

import (
	"bytes"
	"sort"
	"strings"
	"time"

	"github.com/go-zookeeper/zk"
	cmap "github.com/orcaman/concurrent-map"
	"istio.io/api/networking/v1alpha3"
	"istio.io/libistio/pkg/config/event"
)

func (s *Source) ServiceNodeDelete(path string) {
	ss := strings.Split(path, "/")
	service := ss[len(ss)-2]
	if seMap, ok := s.cache.Get(service); ok {
		if ses, ok := seMap.(cmap.ConcurrentMap); ok {
			for serviceKey, value := range ses.Items() {
				if se, ok := value.(*ServiceEntryWithMeta); ok {
					if event, err := buildSeEvent(event.Deleted, se.ServiceEntry, se.Meta, service, nil); err == nil {
						for _, h := range s.handlers {
							h.Handle(event)
						}
					}
					ses.Remove(serviceKey)
				}
			}
		}
		s.cache.Remove(service)
	}
}

func (s *Source) ServiceNodeUpdate(services []string, path string) {
	if !verifyPath(path, false) {
		return
	}
	for _, child := range services {
		go s.Watch(path+"/"+child, s.ServiceNodeUpdate, nil, s.zkGatewayModel)
	}
}

func (s *Source) Watching() {
	s.Watch(s.RegisterRootNode, s.ServiceNodeUpdate, nil, s.zkGatewayModel)
	s.markServiceEntryInitDone()
}

func (s *Source) Watch(path string, updateFunc func([]string, string), deleteFuc func(string), gatewayModel bool) {
	switch len(strings.Split(path, "/")) {
	case 2, 3:
		go s.doWatch(path, updateFunc, deleteFuc)
	case 4:
		if strings.HasPrefix(path, s.RegisterRootNode) {
			if strings.HasSuffix(path, ProviderNode) {
				go s.doWatchInstance(path, s.EndpointUpdate, s.ServiceNodeDelete)
			} else if !gatewayModel && strings.HasSuffix(path, ConsumerNode) {
				go s.doWatchInstance(path, s.EndpointUpdate, nil)
			}
		}
	default:
	}
}

func (s *Source) doWatch(path string, updateFunction func([]string, string), deleteFunction func(string)) {
	if s.watchPath.Has(path) {
		return
	}
	scope.Infof("try to watch the zk path: %q", path)
	initWatch := true
	for {
		children, _, e, err := s.Con.ChildrenW(path)
		if err != nil {
			if shouldStopWatch(err) {
				if !initWatch {
					if deleteFunction != nil {
						deleteFunction(path)
					}
					s.watchPath.Remove(path)
				}
				scope.Warnf("stop watch the zk path %q with err: %v", path, err)
				return
			}
			scope.Errorf("watch the zk path %q failed: %v", path, err)
			time.Sleep(time.Second)
		}
		if initWatch {
			s.watchPath.Set(path, nil)
			scope.Infof("watching the zk path: %q", path)
			initWatch = false
		}
		updateFunction(children, path)
		for watchEvent := range e {
			switch watchEvent.Type {
			case zk.EventNodeDeleted:
				if deleteFunction != nil {
					deleteFunction(path)
				}
				s.watchPath.Remove(path)
				scope.Infof("stop watch the zk path %q because the node was deleted", path)
				return
			case zk.EventResyncWatch:
				scope.Infof("children under the zk path %q resync", path)
				if children, _, err := s.Con.Children(path); err == nil {
					updateFunction(children, path)
				}
			default:
				// ignore other events as the channel will be closed unless the watch events are EventResyncWatch.
				scope.Infof("children under the zk path %q maybe changed, rewatch and update it", path)
			}
		}
	}
}

func (s *Source) doWatchInstance(path string, updateFunction func([]string, []string, string), deleteFunction func(string)) {
	if s.watchPath.Has(path) {
		return
	}
	scope.Infof("try to watch instance under the zk path: %q ", path)
	initWatch := true
	for {
		pathChildren, _, pathEvent, err := s.Con.ChildrenW(path)
		if err != nil {
			if shouldStopWatch(err) {
				if !initWatch {
					if deleteFunction != nil {
						deleteFunction(path)
					}
					s.watchPath.Remove(path)
				}
				scope.Errorf("stop watch instance under the zk path %q with err: %v", path, err)
				return
			}
			scope.Warnf("watch instance under the zk path %q failed: %s", path, err)
			time.Sleep(time.Second)
		}
		if initWatch {
			s.watchPath.Set(path, nil)
			scope.Infof("watching instance under the zk path: %q", path)
			initWatch = false
		}
		provider := false
		providersPath := path[0:len(path)-10] + "/" + ProviderNode
		consumersPath := path
		if strings.HasSuffix(path, ProviderNode) {
			provider = true
			providersPath = path
			consumersPath = path[0:len(path)-10] + "/" + ConsumerNode
		}
		if provider {
			otherChild, _, err := s.Con.Children(consumersPath)
			if err != nil {
				scope.Warnf("get children under the zk path %q failed: %s", consumersPath, err)
			} else {
				updateFunction(pathChildren, otherChild, path)
			}
		} else {
			otherChild, _, err := s.Con.Children(providersPath)
			if err != nil {
				scope.Warnf("get children under the zk path %q failed: %s", consumersPath, err)
			} else {
				updateFunction(otherChild, pathChildren, path)
			}
		}
		for watchEvent := range pathEvent {
			switch watchEvent.Type {
			case zk.EventNodeDeleted:
				if deleteFunction != nil {
					deleteFunction(path)
				}
				s.watchPath.Remove(path)
				scope.Infof("stop watch instance under the zk path %q because the node was deleted", path)
				return
			case zk.EventResyncWatch:
				scope.Infof("instance under the zk path %q resync", path)
				if provider {
					if children, _, err := s.Con.Children(path); err == nil {
						if consumers, _, err := s.Con.Children(consumersPath); err == nil {
							updateFunction(children, consumers, providersPath)
						}
					}
				} else {
					if children, _, err := s.Con.Children(path); err == nil {
						if providers, _, err := s.Con.Children(providersPath); err == nil {
							updateFunction(providers, children, providersPath)
						}
					}
				}
			default:
				// ignore other events as the channel will be closed unless the watch events are EventResyncWatch.
				scope.Infof("instance under the zk path %q maybe changed, rewatch and update instance", path)
			}
		}
	}
}

var shouldStopWatchErrorMsg []string = []string{
	"node does not exist",
}

func shouldStopWatch(err error) bool {
	for idx := range shouldStopWatchErrorMsg {
		if strings.Contains(err.Error(), shouldStopWatchErrorMsg[idx]) {
			return true
		}
	}
	return false
}

func (s *Source) EndpointUpdate(provider, consumer []string, path string) {
	if !verifyPath(path, true) {
		return
	}
	ss := strings.Split(path, "/")
	service := ss[len(ss)-2]

	s.handleServiceData(s.cache, provider, consumer, service, path)
}

func labelToSstring(labels map[string]string) (value string) {
	keys := make([]string, 0)
	for k := range labels {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	var buff bytes.Buffer
	for _, k := range keys {
		buff.WriteString(k)
		buff.WriteString(labels[k])
	}
	return buff.String()
}

func sortEndpoint(endpoints []*v1alpha3.WorkloadEntry) {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Address == endpoints[j].Address {
			return labelToSstring(endpoints[i].Labels) < labelToSstring(endpoints[j].Labels)
		}
		return endpoints[i].Address < endpoints[j].Address
	})
}

func verifyPath(path string, isInstance bool) bool {
	ss := strings.Split(path, "/")
	if len(ss) < 2 {
		scope.Errorf("Invalid watch path")
		return false
	}
	if isInstance {
		if ss[len(ss)-1] != ConsumerNode && ss[len(ss)-1] != ProviderNode {
			scope.Errorf("Invalid watch path")
			return false
		}
	}
	return true
}
