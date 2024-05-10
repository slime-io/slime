package main

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/yaml"
)

var configDataName = "cfg"

type controller struct {
	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller
}

func NewController(
	indexer cache.Indexer,
	queue workqueue.RateLimitingInterface,
	informer cache.Controller,
) *controller {
	return &controller{
		indexer:  indexer,
		queue:    queue,
		informer: informer,
	}
}

func (c *controller) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()

	defer c.queue.ShutDown()
	log.Info("Starting configmap controller")

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh
	log.Info("Stopping configmap controller")
}

func (c *controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	log.Debugf("Process item %q", key)
	err := c.updatePorts(key.(string))
	if err != nil {
		log.Warnf("Update ports with err: %v, retry after", err)
		c.queue.AddRateLimited(key)
	} else {
		c.queue.Forget(key)
	}
	return true
}

func (c *controller) updatePorts(key string) error {
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		return fmt.Errorf("Fetching object with key %s from store failed with: %v", key, err)
	}

	var wormholePorts map[int]struct{}
	if !exists {
		log.Infof("Configmap %s does not exist anymore", key)
	} else {
		// TODO how to handle del event
		cm, err := convertToConfigMap(obj)
		if err != nil {
			return fmt.Errorf("Convert to configmap failed with: %v", err)
		}
		wormholePorts = extractWormholePorts(cm.Data[configDataName])
	}
	startListenAndServe(wormholePorts)
	return nil
}

func convertToConfigMap(obj interface{}) (*corev1.ConfigMap, error) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return nil, fmt.Errorf("couldn't get object from tombstone %#v", obj)
		}
		cm, ok = tombstone.Obj.(*corev1.ConfigMap)
		if !ok {
			return nil, fmt.Errorf("tombstone contained object that is not a ConfigMap %#v", obj)
		}
	}
	return cm, nil
}

func extractWormholePorts(rawCfg string) map[int]struct{} {
	log.Debugf("ExtractWormholePorts with rawCfg: %q", rawCfg)
	ctr := struct {
		WormholePorts []int
	}{}
	err := yaml.Unmarshal([]byte(rawCfg), &ctr)
	if err != nil {
		log.Warnf("Unmarshal %s falied: %v", rawCfg, err)
		return nil
	}
	ret := map[int]struct{}{}
	for _, port := range ctr.WormholePorts {
		ret[port] = struct{}{}
	}
	return ret
}
