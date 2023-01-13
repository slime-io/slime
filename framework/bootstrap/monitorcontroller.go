package bootstrap

import (
	"errors"
	log "github.com/sirupsen/logrus"
	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap/resource"
	"sync"
)

type monitorController struct {
	monitor      Monitor
	configStore  ConfigStore
	kind         monitorControllerKind
	configSource *bootconfig.ConfigSource
	ready        bool
	sync.RWMutex
}

type monitorControllerKind int

const (
	Istio monitorControllerKind = iota
	Kubernetes
	McpOverXds
)

func newMonitorController(cs ConfigStore) *monitorController {
	out := &monitorController{
		configStore: cs,
		monitor:     newMonitor(cs),
	}
	return out
}

func (c *monitorController) SetReady() {
	c.Lock()
	c.ready = true
	c.Unlock()
}

func (c *monitorController) RegisterEventHandler(kind resource.GroupVersionKind, f func(resource.Config, resource.Config, Event)) {
	c.monitor.AppendEventHandler(kind, f)
}

func (c *monitorController) InitReady() bool {
	c.RLock()
	ready := c.ready
	c.RUnlock()
	return ready
}

func (c *monitorController) Run(stop <-chan struct{}) {
	c.monitor.Run(stop)
}

func (c *monitorController) Schemas() resource.Schemas {
	return c.configStore.Schemas()
}

func (c *monitorController) Create(config resource.Config) (revision string, err error) {
	if revision, err = c.configStore.Create(config); err == nil {
		c.monitor.ScheduleProcessEvent(ConfigEvent{
			config: config,
			event:  EventAdd,
		})
		log.Debugf("ConfigEvent EventAdd: [%s] %s/%s", config.GroupVersionKind, config.Namespace, config.Name)
	}
	return
}

func (c *monitorController) Update(config resource.Config) (newRevision string, err error) {
	oldConfig := c.configStore.Get(config.GroupVersionKind, config.Name, config.Namespace)
	if newRevision, err = c.configStore.Update(config); err == nil && newRevision != oldConfig.ResourceVersion {
		c.monitor.ScheduleProcessEvent(ConfigEvent{
			old:    *oldConfig,
			config: config,
			event:  EventUpdate,
		})
		log.Debugf("ConfigEvent EventUpdate: [%s] %s/%s", config.GroupVersionKind, config.Namespace, config.Name)
	}
	return
}

func (c *monitorController) Delete(kind resource.GroupVersionKind, key, namespace string) (err error) {
	if config := c.Get(kind, key, namespace); config != nil {
		if err = c.configStore.Delete(kind, key, namespace); err == nil {
			c.monitor.ScheduleProcessEvent(ConfigEvent{
				config: *config,
				event:  EventDelete,
			})
			log.Debugf("ConfigEvent EventDelete: [%s] %s/%s", config.GroupVersionKind, config.Namespace, config.Name)
			return
		}
	}
	return errors.New("Delete failure: config " + key + " " + namespace + " does not exist")
}

func (c *monitorController) Get(kind resource.GroupVersionKind, key, namespace string) *resource.Config {
	return c.configStore.Get(kind, key, namespace)
}

func (c *monitorController) List(kind resource.GroupVersionKind, namespace string) ([]resource.Config, error) {
	return c.configStore.List(kind, namespace)
}
