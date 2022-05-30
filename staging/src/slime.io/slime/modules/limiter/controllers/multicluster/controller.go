/*
* @Author: yangdihang
* @Date: 2020/7/24
 */

package multicluster

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"slime.io/slime/framework/bootstrap"
)

const (
	RootNamespace = "istio-mc"
	maxRetries    = 3
)

type Controller struct {
	k8sClient    *kubernetes.Clientset
	informer     cache.SharedIndexInformer
	queue        workqueue.RateLimitingInterface
	cs           map[string]string
	subscriber   []func(*kubernetes.Clientset)
	unSubscriber []func(string)
	stop         <-chan struct{}
}

func New(env *bootstrap.Environment, subscriber []func(*kubernetes.Clientset), unSubscriber []func(string)) *Controller {
	c := &Controller{
		k8sClient:    env.K8SClient,
		stop:         env.Stop,
		subscriber:   subscriber,
		unSubscriber: unSubscriber,
		cs:           make(map[string]string),
	}

	secretsInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts meta_v1.ListOptions) (runtime.Object, error) {
				opts.LabelSelector = env.Config.Global.Multicluster + "=true"
				obj, err := c.k8sClient.CoreV1().Secrets(RootNamespace).List(opts)
				return obj, err
			},
			WatchFunc: func(opts meta_v1.ListOptions) (watch.Interface, error) {
				opts.LabelSelector = env.Config.Global.Multicluster + "=true"
				return c.k8sClient.CoreV1().Secrets(RootNamespace).Watch(opts)
			},
		},
		&corev1.Secret{}, 0, cache.Indexers{},
	)
	c.informer = secretsInformer

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	c.queue = queue

	log.Info("Setting up event handlers")
	secretsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			log.Infof("Processing add: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			log.Infof("Processing delete: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
	})
	return c
}

func (c *Controller) Run() {
	defer c.queue.ShutDown()

	log.Info("Starting Secrets controller")

	go c.informer.Run(c.stop)

	// Wait for the caches to be synced before starting workers
	log.Info("Waiting for informer caches to sync")
	if !cache.WaitForCacheSync(c.stop, c.informer.HasSynced) {
		// utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}
	wait.Until(c.runWorker, 5*time.Second, c.stop)
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	secretName, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(secretName)

	err := c.processItem(secretName.(string))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(secretName)
	} else if c.queue.NumRequeues(secretName) < maxRetries {
		log.Errorf("processing %s err, (will retry): %+v", secretName, err)
		c.queue.AddRateLimited(secretName)
	} else {
		log.Errorf("processing %s err, (giving up): %+v", secretName, err)
		c.queue.Forget(secretName)
	}

	return true
}

func (c *Controller) processItem(secretName string) error {
	obj, exists, err := c.informer.GetIndexer().GetByKey(secretName)
	if err != nil {
		return fmt.Errorf("error fetching object %s error: %v", secretName, err)
	}

	if exists {
		c.addMemberCluster(secretName, obj.(*corev1.Secret))
	} else {
		c.deleteMemberCluster(secretName)
	}

	return nil
}

func (c *Controller) addMemberCluster(secretName string, s *corev1.Secret) {
	for clusterID, kubeConfig := range s.Data {
		// clusterID must be unique even across multiple secrets
		if _, ok := c.cs[clusterID]; !ok {
			if len(kubeConfig) == 0 {
				log.Infof("Data '%s' in the secret %s in namespace %s is empty, and disregarded ",
					clusterID, secretName, s.Namespace)
				continue
			}

			clientConfig, err := clientcmd.Load(kubeConfig)
			if err != nil {
				log.Infof("Data '%s' in the secret %s in namespace %s is not a kubeconfig: %+v",
					clusterID, secretName, s.Namespace, err)
				continue
			}

			if err := clientcmd.Validate(*clientConfig); err != nil {
				log.Errorf("Data '%s' in the secret %s in namespace %s is not a valid kubeconfig: %+v",
					clusterID, secretName, s.Namespace, err)
				continue
			}

			log.Infof("Adding new cluster member: %s", clusterID)
			c.cs[clusterID] = secretName
			client := clientcmd.NewDefaultClientConfig(*clientConfig, &clientcmd.ConfigOverrides{})
			restConfig, err := client.ClientConfig()
			if err != nil {
				continue
			}
			k8sClient, err := kubernetes.NewForConfig(restConfig)
			if err != nil {
				log.Errorf("error during create of kubernetes client interface for cluster: %s %+v", clusterID, err)
				continue
			}
			for _, f := range c.subscriber {
				f(k8sClient)
			}
		} else {
			log.Infof("Cluster %s in the secret %s in namespace %s already exists",
				clusterID, c.cs[clusterID], s.Namespace)
		}
	}
	log.Infof("Number of remote clusters: %d", len(c.cs))
}

func (c *Controller) deleteMemberCluster(secretName string) {
	for clusterID, cluster := range c.cs {
		if cluster == secretName {
			log.Infof("Deleting cluster member :%s", clusterID)
			for _, f := range c.unSubscriber {
				f(clusterID)
			}
			delete(c.cs, clusterID)
		}
	}
	log.Infof("Number of remote clusters: %d", len(c.cs))
}
