package main

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func startSvcCache(ctx context.Context) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Services("").Watch(ctx, metav1.ListOptions{})
		},
	}

	_, controller := cache.NewInformer(lw, &corev1.Service{}, 60, cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { handleSvcUpdate(nil, obj) },
		UpdateFunc: func(oldObj, newObj interface{}) { handleSvcUpdate(oldObj, newObj) },
		DeleteFunc: func(obj interface{}) { handleSvcDelete(obj) },
	})

	go controller.Run(nil)

	if !cache.WaitForCacheSync(ctx.Done(), controller.HasSynced) {
		return fmt.Errorf("failed to wait for service cache sync")
	}

	return nil
}

func handleSvcUpdate(old, obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	nn := types.NamespacedName{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}

	Cache.Set(nn)
}

func handleSvcDelete(obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	nn := types.NamespacedName{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}
	Cache.Delete(nn)
}
