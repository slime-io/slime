package bootstrap

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	networkingapi "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioclientgo "istio.io/client-go/pkg/clientset/versioned"
	istioinformers "istio.io/client-go/pkg/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	bootconfig "slime.io/slime/framework/apis/config/v1alpha1"
	"slime.io/slime/framework/bootstrap/collections"
	"slime.io/slime/framework/bootstrap/resource"
)

// TODO support multi clusters, configSource takes no effect now
func initK8sMonitorController(configSource *bootconfig.ConfigSource) (*monitorController, error) {
	kcs := makeConfigStore(collections.Kube)
	mc := newMonitorController(kcs)
	mc.kind = Kubernetes
	mc.configSource = configSource
	log.Infof("init k8s config source successfully")
	return mc, nil
}

func startK8sMonitorController(mc *monitorController, cfg *rest.Config, stopCh <-chan struct{}) error {
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to init k8s client sets for K8S config source: %+v", err)
	}
	informerFactory := informers.NewSharedInformerFactory(clientSet, 0)

	istioclientSet, err := istioclientgo.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to init istio client sets for K8S config source: %+v", err)
	}
	istioInformerFactory := istioinformers.NewSharedInformerFactory(istioclientSet, 0)

	type buildHelper struct {
		getInformer  func(informers.SharedInformerFactory, istioinformers.SharedInformerFactory) cache.SharedIndexInformer
		eventHandler cache.ResourceEventHandlerFuncs
	}
	var builders []*buildHelper = []*buildHelper{
		{
			// pod
			getInformer: func(i informers.SharedInformerFactory, _ istioinformers.SharedInformerFactory) cache.SharedIndexInformer {
				return i.Core().V1().Pods().Informer()
			},
			eventHandler: cache.ResourceEventHandlerFuncs{
				AddFunc:    addFuncFactory[*corev1.Pod](mc, resource.Pod),
				UpdateFunc: updateFuncFactory[*corev1.Pod](mc, resource.Pod),
				DeleteFunc: deleteFuncFactory[*corev1.Pod](mc, resource.Pod),
			},
		},
		{
			// ep
			getInformer: func(i informers.SharedInformerFactory, _ istioinformers.SharedInformerFactory) cache.SharedIndexInformer {
				return i.Core().V1().Endpoints().Informer()
			},
			eventHandler: cache.ResourceEventHandlerFuncs{
				AddFunc:    addFuncFactory[*corev1.Endpoints](mc, resource.Endpoints),
				UpdateFunc: updateFuncFactory[*corev1.Endpoints](mc, resource.Endpoints),
				DeleteFunc: deleteFuncFactory[*corev1.Endpoints](mc, resource.Endpoints),
			},
		},
		{
			// svc
			getInformer: func(i informers.SharedInformerFactory, _ istioinformers.SharedInformerFactory) cache.SharedIndexInformer {
				return i.Core().V1().Services().Informer()
			},
			eventHandler: cache.ResourceEventHandlerFuncs{
				AddFunc:    addFuncFactory[*corev1.Service](mc, resource.Service),
				UpdateFunc: updateFuncFactory[*corev1.Service](mc, resource.Service),
				DeleteFunc: deleteFuncFactory[*corev1.Service](mc, resource.Service),
			},
		},
		{
			// cm
			getInformer: func(i informers.SharedInformerFactory, _ istioinformers.SharedInformerFactory) cache.SharedIndexInformer {
				return i.Core().V1().ConfigMaps().Informer()
			},
			eventHandler: cache.ResourceEventHandlerFuncs{
				AddFunc:    addFuncFactory[*corev1.ConfigMap](mc, resource.ConfigMap),
				UpdateFunc: updateFuncFactory[*corev1.ConfigMap](mc, resource.ConfigMap),
				DeleteFunc: deleteFuncFactory[*corev1.ConfigMap](mc, resource.ConfigMap),
			},
		},
		{
			// serviceEntry
			getInformer: func(_ informers.SharedInformerFactory, i istioinformers.SharedInformerFactory) cache.SharedIndexInformer {
				return i.Networking().V1alpha3().ServiceEntries().Informer()
			},
			eventHandler: cache.ResourceEventHandlerFuncs{
				AddFunc:    addFuncFactory[*networkingapi.ServiceEntry](mc, resource.ServiceEntry),
				UpdateFunc: updateFuncFactory[*networkingapi.ServiceEntry](mc, resource.ServiceEntry),
				DeleteFunc: deleteFuncFactory[*networkingapi.ServiceEntry](mc, resource.ServiceEntry),
			},
		},
	}

	hasSyncs := make([]func() bool, 0, len(builders))
	for _, b := range builders {
		informer := b.getInformer(informerFactory, istioInformerFactory)
		_, _ = informer.AddEventHandler(b.eventHandler)
		hasSyncs = append(hasSyncs, informer.HasSynced)
	}

	informerFactory.Start(stopCh)
	istioInformerFactory.Start(stopCh)
	for _, hasSynced := range hasSyncs {
		cache.WaitForCacheSync(stopCh, hasSynced)
	}

	mc.SetReady()
	log.Infof("start K8S config source successfully")
	return nil
}

func addFuncFactory[T metav1.Object](mc *monitorController, gvk resource.GroupVersionKind) func(obj interface{}) {
	return func(obj interface{}) {
		re, ok := obj.(T)
		if !ok {
			return
		}
		config := resource.Config{
			Meta: resource.Meta{
				GroupVersionKind:  gvk,
				Name:              re.GetName(),
				Namespace:         re.GetNamespace(),
				Labels:            re.GetLabels(),
				Annotations:       re.GetAnnotations(),
				ResourceVersion:   re.GetResourceVersion(),
				CreationTimestamp: re.GetCreationTimestamp().Time,
			},
			Spec: re,
		}
		if _, err := mc.Create(config); err != nil {
			log.Errorf("monitor controller create err: %+v", err)
		}
	}
}

func updateFuncFactory[T metav1.Object](mc *monitorController, gvk resource.GroupVersionKind) func(oldObj, newObj interface{}) {
	return func(_, obj interface{}) {
		re, ok := obj.(T)
		if !ok {
			return
		}
		config := resource.Config{
			Meta: resource.Meta{
				GroupVersionKind:  gvk,
				Name:              re.GetName(),
				Namespace:         re.GetNamespace(),
				Labels:            re.GetLabels(),
				Annotations:       re.GetAnnotations(),
				ResourceVersion:   re.GetResourceVersion(),
				CreationTimestamp: re.GetCreationTimestamp().Time,
			},
			Spec: re,
		}
		if _, err := mc.Update(config); err != nil {
			log.Errorf("monitor controller update err: %+v", err)
		}
	}
}

func deleteFuncFactory[T metav1.Object](mc *monitorController, gvk resource.GroupVersionKind) func(obj interface{}) {
	return func(obj interface{}) {
		re, ok := obj.(T)
		if !ok {
			return
		}
		if err := mc.Delete(gvk, re.GetName(), re.GetNamespace()); err != nil {
			log.Errorf("monitor controller delete err: %+v", err)
		}
	}
}
