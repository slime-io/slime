package multicluster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "multicluster")

const (
	multiClusterSecretLabel = "istio/multiCluster"
	maxRetries              = 5
)

type ClusterHandler interface {
	ClusterAdded(cluster *Cluster, stop <-chan struct{}) error
	ClusterUpdated(cluster *Cluster, stop <-chan struct{}) error
	ClusterDeleted(clusterID string) error
}

// Controller is the controller implementation for Secret resources
type Controller struct {
	localClusterID     string
	localClusterClient kubernetes.Interface
	queue              workqueue.RateLimitingInterface
	informer           cache.SharedIndexInformer
	cs                 *ClusterStore
	handlers           []ClusterHandler
	syncInterval       time.Duration

	resyncPeriod time.Duration
}

// NewController returns a new secret controller
func NewController(kubeClient kubernetes.Interface, namespace string, resyncPeriod time.Duration, localClusterID string) *Controller {
	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.LabelSelector = multiClusterSecretLabel + "=true"
			return kubeClient.CoreV1().Secrets(namespace).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.LabelSelector = multiClusterSecretLabel + "=true"
			return kubeClient.CoreV1().Secrets(namespace).Watch(context.TODO(), opts)
		},
	}
	informer := cache.NewSharedIndexInformer(lw, &corev1.Secret{}, resyncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	controller := &Controller{
		resyncPeriod:       resyncPeriod,
		localClusterID:     localClusterID,
		localClusterClient: kubeClient,
		cs:                 newClustersStore(),
		informer:           informer,
		queue:              queue,
		syncInterval:       100 * time.Millisecond,
	}

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				log.Infof("Processing add: %s", key)
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if oldObj == newObj || reflect.DeepEqual(oldObj, newObj) {
				return
			}

			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				log.Infof("Processing update: %s", key)
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				log.Infof("Processing delete: %s", key)
				queue.Add(key)
			}
		},
	})

	return controller
}

func (c *Controller) AddHandler(h ClusterHandler) {
	log.Infof("handling remote clusters in %T", h)
	c.handlers = append(c.handlers, h)
}

// Run starts the controller until it receives a message over stopCh
func (c *Controller) Run(stopCh <-chan struct{}) error {
	localCluster := &Cluster{
		ID:           c.localClusterID,
		KubeClient:   c.localClusterClient,
		KubeInformer: informers.NewSharedInformerFactory(c.localClusterClient, c.resyncPeriod),
	}
	if err := c.handleAdd(localCluster, stopCh); err != nil {
		return fmt.Errorf("failed initializing local cluster %s: %v", c.localClusterID, err)
	}
	go func() {
		defer utilruntime.HandleCrash()
		defer c.queue.ShutDown()

		t0 := time.Now()
		log.Info("Starting multicluster remote secrets controller")

		go c.informer.Run(stopCh)

		if err := wait.PollImmediateUntil(c.syncInterval, func() (bool, error) {
			if !c.informer.HasSynced() {
				return false, nil
			}
			return true, nil
		}, stopCh); err != nil {
			log.Error("Failed to sync multicluster remote secrets controller cache")
			return
		}
		log.Infof("multicluster remote secrets controller cache synced in %v", time.Since(t0))

		go wait.Until(c.runWorker, 5*time.Second, stopCh)
		<-stopCh
		c.close()
	}()
	return nil
}

func (c *Controller) close() {
	c.cs.Lock()
	defer c.cs.Unlock()
	for _, clusterMap := range c.cs.remoteClusters {
		for _, cluster := range clusterMap {
			close(cluster.stop)
		}
	}
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		log.Info("secret controller queue is shutting down, so returning")
		return false
	}
	log.Infof("secret controller got event from queue for secret %s", key)
	defer c.queue.Done(key)

	err := c.processItem(key.(string))
	if err == nil {
		log.Debugf("secret controller finished processing secret %s", key)
		// No error, reset the ratelimit counters
		c.queue.Forget(key)
	} else if c.queue.NumRequeues(key) < maxRetries {
		log.Errorf("Error processing %s (will retry): %v", key, err)
		c.queue.AddRateLimited(key)
	} else {
		log.Errorf("Error processing %s (giving up): %v", key, err)
		c.queue.Forget(key)
	}

	return true
}

func (c *Controller) processItem(key string) error {
	log.Infof("processing secret event for secret %s", key)
	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("error fetching object %s error: %v", key, err)
	}
	if exists {
		log.Debugf("secret %s exists in informer cache, processing it", key)
		c.addSecret(key, obj.(*corev1.Secret))
	} else {
		log.Debugf("secret %s does not exist in informer cache, deleting it", key)
		c.deleteSecret(key)
	}

	return nil
}

func (c *Controller) addSecret(secretKey string, s *corev1.Secret) {
	// First delete clusters
	existingClusters := c.cs.GetExistingClustersFor(secretKey)
	for _, existingCluster := range existingClusters {
		if _, ok := s.Data[string(existingCluster.ID)]; !ok {
			c.deleteCluster(secretKey, existingCluster.ID)
		}
	}

	for clusterID, kubeConfig := range s.Data {
		action, callback := "Adding", c.handleAdd
		if prev := c.cs.Get(secretKey, clusterID); prev != nil {
			action, callback = "Updating", c.handleUpdate
			kubeConfigSha := sha256.Sum256(kubeConfig)
			if bytes.Equal(kubeConfigSha[:], prev.kubeConfigSha[:]) {
				log.Infof("skipping update of cluster_id=%v from secret=%v: (kubeconfig are identical)", clusterID, secretKey)
				continue
			}
		}
		if clusterID == c.localClusterID {
			log.Infof("ignoring %s cluster %v from secret %v as it would overwrite the local cluster", action, clusterID, secretKey)
			continue
		}
		log.Infof("%s cluster %v from secret %v", action, clusterID, secretKey)

		remoteCluster, err := c.createRemoteCluster(kubeConfig, clusterID)
		if err != nil {
			log.Errorf("%s cluster_id=%v from secret=%v: %v", action, clusterID, secretKey, err)
			continue
		}
		c.cs.Store(secretKey, remoteCluster.ID, remoteCluster)
		if err := callback(remoteCluster, remoteCluster.stop); err != nil {
			log.Errorf("%s cluster_id from secret=%v: %s %v", action, clusterID, secretKey, err)
			continue
		}
		log.Infof("finished callback for %s", clusterID)
	}

	log.Infof("Number of remote clusters: %d", c.cs.Len())
}

func (c *Controller) deleteSecret(secretKey string) {
	c.cs.Lock()
	defer func() {
		c.cs.Unlock()
		log.Infof("Number of remote clusters: %d", c.cs.Len())
	}()
	for clusterID, cluster := range c.cs.remoteClusters[secretKey] {
		if clusterID == c.localClusterID {
			log.Infof("ignoring delete cluster %v from secret %v as it would overwrite the local cluster", clusterID, secretKey)
			continue
		}
		log.Infof("Deleting cluster_id=%v configured by secret=%v", clusterID, secretKey)
		err := c.handleDelete(clusterID)
		if err != nil {
			log.Errorf("Error removing cluster_id=%v configured by secret=%v: %v",
				clusterID, secretKey, err)
		}
		close(cluster.stop)
		delete(c.cs.remoteClusters[secretKey], clusterID)
	}
	delete(c.cs.remoteClusters, secretKey)
}

// BuildClientsFromConfig creates kubernetes.Interface from the provided kubeconfig.
func BuildClientsFromConfig(kubeConfig []byte) (kubernetes.Interface, error) {
	if len(kubeConfig) == 0 {
		return nil, errors.New("kubeconfig is empty")
	}

	rawConfig, err := clientcmd.Load(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig cannot be loaded: %v", err)
	}

	if err := clientcmd.Validate(*rawConfig); err != nil {
		return nil, fmt.Errorf("kubeconfig is not valid: %v", err)
	}

	clientConfig := clientcmd.NewDefaultClientConfig(*rawConfig, &clientcmd.ConfigOverrides{})
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get rest config from kubeconfig: %v", err)
	}
	return kubernetes.NewForConfig(restConfig)
}

func (c *Controller) createRemoteCluster(kubeConfig []byte, ID string) (*Cluster, error) {
	clients, err := BuildClientsFromConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	informers := informers.NewSharedInformerFactory(clients, c.resyncPeriod)
	return &Cluster{
		ID:            ID,
		KubeClient:    clients,
		KubeInformer:  informers,
		stop:          make(chan struct{}),
		kubeConfigSha: sha256.Sum256(kubeConfig),
	}, nil
}

func (c *Controller) deleteCluster(secretKey string, clusterID string) {
	c.cs.Lock()
	defer func() {
		c.cs.Unlock()
		log.Infof("Number of remote clusters: %d", c.cs.Len())
	}()
	log.Infof("Deleting cluster_id=%v configured by secret=%v", clusterID, secretKey)
	err := c.handleDelete(clusterID)
	if err != nil {
		log.Errorf("Error removing cluster_id=%v configured by secret=%v: %v",
			clusterID, secretKey, err)
	}
	close(c.cs.remoteClusters[secretKey][clusterID].stop)
	delete(c.cs.remoteClusters[secretKey], clusterID)
}

func (c *Controller) handleAdd(cluster *Cluster, stop <-chan struct{}) error {
	var errs *multierror.Error
	for _, handler := range c.handlers {
		errs = multierror.Append(errs, handler.ClusterAdded(cluster, stop))
	}
	return errs.ErrorOrNil()
}

func (c *Controller) handleUpdate(cluster *Cluster, stop <-chan struct{}) error {
	var errs *multierror.Error
	for _, handler := range c.handlers {
		errs = multierror.Append(errs, handler.ClusterUpdated(cluster, stop))
	}
	return errs.ErrorOrNil()
}

func (c *Controller) handleDelete(key string) error {
	var errs *multierror.Error
	for _, handler := range c.handlers {
		errs = multierror.Append(errs, handler.ClusterDeleted(key))
	}
	return errs.ErrorOrNil()
}

// Cluster defines cluster struct
type Cluster struct {
	ID string

	KubeClient   kubernetes.Interface
	KubeInformer informers.SharedInformerFactory

	kubeConfigSha [sha256.Size]byte
	stop          chan struct{}
}

// ClusterStore is a collection of clusters
type ClusterStore struct {
	sync.RWMutex
	// keyed by secret key(ns/name)->clusterID
	remoteClusters map[string]map[string]*Cluster
}

// newClustersStore initializes data struct to store clusters information
func newClustersStore() *ClusterStore {
	return &ClusterStore{
		remoteClusters: make(map[string]map[string]*Cluster),
	}
}

func (c *ClusterStore) Get(secretKey string, clusterID string) *Cluster {
	c.RLock()
	defer c.RUnlock()
	if _, ok := c.remoteClusters[secretKey]; !ok {
		return nil
	}
	return c.remoteClusters[secretKey][clusterID]
}

// GetExistingClustersFor return existing clusters registered for the given secret
func (c *ClusterStore) GetExistingClustersFor(secretKey string) []*Cluster {
	c.RLock()
	defer c.RUnlock()
	out := make([]*Cluster, 0, len(c.remoteClusters[secretKey]))
	for _, cluster := range c.remoteClusters[secretKey] {
		out = append(out, cluster)
	}
	return out
}

func (c *ClusterStore) Store(secretKey string, clusterID string, value *Cluster) {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.remoteClusters[secretKey]; !ok {
		c.remoteClusters[secretKey] = make(map[string]*Cluster)
	}
	c.remoteClusters[secretKey][clusterID] = value
}

func (c *ClusterStore) Len() int {
	c.Lock()
	defer c.Unlock()
	out := 0
	for _, clusterMap := range c.remoteClusters {
		out += len(clusterMap)
	}
	return out
}
