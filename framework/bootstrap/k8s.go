package bootstrap

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	networkingapi "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
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
		return fmt.Errorf("failed to init client for K8S config source: %+v", err)
	}

	// svc的handler处理时需要ep信息，ep处理时需要pod信息
	// 需要保证启用顺序：pod -> ep -> svc
	podInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	podInformer := podInformerFactory.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.Pod)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Pod,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Create(config); err != nil {
				log.Errorf("monitor controller create err: %+v", err)
			}
		},
		UpdateFunc: func(_, new interface{}) {
			re, ok := new.(*corev1.Pod)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Pod,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Update(config); err != nil {
				log.Errorf("monitor controller update err: %+v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.Pod)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Pod,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if err := mc.Delete(config.GroupVersionKind, config.Name, config.Namespace); err != nil {
				log.Errorf("monitor controller delete err: %+v", err)
			}
		},
	})
	podInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, podInformer.HasSynced)

	epInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	epInformer := epInformerFactory.Core().V1().Endpoints().Informer()
	epInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.Endpoints)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Endpoints,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Create(config); err != nil {
				log.Errorf("monitor controller create err: %+v", err)
			}
		},
		UpdateFunc: func(_, new interface{}) {
			re, ok := new.(*corev1.Endpoints)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Endpoints,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Update(config); err != nil {
				log.Errorf("monitor controller update err: %+v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.Endpoints)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Endpoints,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if err := mc.Delete(config.GroupVersionKind, config.Name, config.Namespace); err != nil {
				log.Errorf("monitor controller delete err: %+v", err)
			}
		},
	})
	epInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, epInformer.HasSynced)

	svcInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	svcInformer := svcInformerFactory.Core().V1().Services().Informer()
	svcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.Service)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Service,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Create(config); err != nil {
				log.Errorf("monitor controller create err: %+v", err)
			}
		},
		UpdateFunc: func(_, new interface{}) {
			re, ok := new.(*corev1.Service)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Service,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Update(config); err != nil {
				log.Errorf("monitor controller update err: %+v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.Service)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.Service,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if err := mc.Delete(config.GroupVersionKind, config.Name, config.Namespace); err != nil {
				log.Errorf("monitor controller delete err: %+v", err)
			}
		},
	})
	svcInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, svcInformer.HasSynced)

	cmInformerFactory := informers.NewSharedInformerFactory(clientSet, 0)
	cmInformer := cmInformerFactory.Core().V1().ConfigMaps().Informer()
	cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.ConfigMap,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Create(config); err != nil {
				log.Errorf("monitor controller create err: %+v", err)
			}
		},
		UpdateFunc: func(_, new interface{}) {
			re, ok := new.(*corev1.ConfigMap)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.ConfigMap,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if _, err := mc.Update(config); err != nil {
				log.Errorf("monitor controller update err: %+v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			re, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.ConfigMap,
					Name:              re.Name,
					Namespace:         re.Namespace,
					Labels:            re.Labels,
					Annotations:       re.Annotations,
					ResourceVersion:   re.ResourceVersion,
					CreationTimestamp: re.CreationTimestamp.Time,
				},
				Spec: re,
			}
			if err := mc.Delete(config.GroupVersionKind, config.Name, config.Namespace); err != nil {
				log.Errorf("monitor controller delete err: %+v", err)
			}
		},
	})
	cmInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, cmInformer.HasSynced)

	gvr, _ := schema.ParseResourceArg("serviceentries.v1alpha3.networking.istio.io")

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to init client for K8S config source: %+v", err)
	}

	seInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 0, "", nil)
	seInformer := seInformerFactory.ForResource(*gvr).Informer()
	seInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			re, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			spec, _, _ := unstructured.NestedFieldCopy(re.Object, "spec")
			var serviceEntry networkingapi.ServiceEntry
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(spec.(map[string]interface{}), &serviceEntry); err != nil {
				log.Errorf("convert ServiceEntry %s/%s to structured error: %v", re.GetNamespace(), re.GetName(), err)
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.ServiceEntry,
					Name:              re.GetName(),
					Namespace:         re.GetNamespace(),
					Labels:            re.GetLabels(),
					Annotations:       re.GetAnnotations(),
					ResourceVersion:   re.GetResourceVersion(),
					CreationTimestamp: time.Time{},
				},
				Spec: &serviceEntry,
			}
			if _, err := mc.Create(config); err != nil {
				log.Errorf("monitor controller create err: %+v", err)
			}
		},
		UpdateFunc: func(_, new interface{}) {
			re, ok := new.(*unstructured.Unstructured)
			if !ok {
				return
			}
			spec, _, _ := unstructured.NestedFieldCopy(re.Object, "spec")
			var serviceEntry networkingapi.ServiceEntry
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(spec.(map[string]interface{}), &serviceEntry); err != nil {
				log.Errorf("convert ServiceEntry %s/%s to structured error: %v", re.GetNamespace(), re.GetName(), err)
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.ServiceEntry,
					Name:              re.GetName(),
					Namespace:         re.GetNamespace(),
					Labels:            re.GetLabels(),
					Annotations:       re.GetAnnotations(),
					ResourceVersion:   re.GetResourceVersion(),
					CreationTimestamp: time.Time{},
				},
				Spec: &serviceEntry,
			}
			if _, err := mc.Update(config); err != nil {
				log.Errorf("monitor controller update err: %+v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			re, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			spec, _, _ := unstructured.NestedFieldCopy(re.Object, "spec")
			var serviceEntry networkingapi.ServiceEntry
			if err = runtime.DefaultUnstructuredConverter.FromUnstructured(spec.(map[string]interface{}), &serviceEntry); err != nil {
				log.Errorf("convert ServiceEntry %s/%s to structured error: %v", re.GetNamespace(), re.GetName(), err)
				return
			}
			config := resource.Config{
				ConfigMeta: resource.ConfigMeta{
					GroupVersionKind:  resource.ServiceEntry,
					Name:              re.GetName(),
					Namespace:         re.GetNamespace(),
					Labels:            re.GetLabels(),
					Annotations:       re.GetAnnotations(),
					ResourceVersion:   re.GetResourceVersion(),
					CreationTimestamp: time.Time{},
				},
				Spec: &serviceEntry,
			}
			if err := mc.Delete(config.GroupVersionKind, config.Name, config.Namespace); err != nil {
				log.Errorf("monitor controller delete err: %+v", err)
			}
		},
	})
	seInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, seInformer.HasSynced)

	mc.SetReady()
	log.Infof("start K8S config source successfully")
	return nil
}
