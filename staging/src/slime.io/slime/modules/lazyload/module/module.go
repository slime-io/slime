package module

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	istioapi "slime.io/slime/framework/apis"
	basecontroller "slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model/metric"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/modules/lazyload/api/config"
	lazyloadapiv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
	"slime.io/slime/modules/lazyload/controllers"
	modmodel "slime.io/slime/modules/lazyload/model"
	"slime.io/slime/modules/lazyload/pkg/server"
)

var log = modmodel.ModuleLog

type Module struct {
	config config.Fence
}

func (m *Module) Kind() string {
	return modmodel.ModuleName
}

func (m *Module) Config() proto.Message {
	return &m.config
}

func (m *Module) InitScheme(scheme *runtime.Scheme) error {
	for _, f := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		lazyloadapiv1alpha1.AddToScheme,
		istioapi.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) Clone() module.Module {
	ret := *m
	return &ret
}

func (m *Module) Setup(opts module.ModuleOptions) error {
	log.Debugf("lazyload setup begin")

	env, mgr, le := opts.Env, opts.Manager, opts.LeaderElectionCbs
	pc, err := controllers.NewProducerConfig(env, m.config)
	if err != nil {
		return fmt.Errorf("unable to create ProducerConfig, %+v", err)
	}
	sfReconciler := controllers.NewReconciler(
		controllers.ReconcilerWithCfg(&m.config),
		controllers.ReconcilerWithEnv(env),
		controllers.ReconcilerWithProducerConfig(pc),
	)
	sfReconciler.Client = mgr.GetClient()
	sfReconciler.Scheme = mgr.GetScheme()

	if env.ConfigController != nil {
		sfReconciler.RegisterSeHandler()
	}

	podNs := os.Getenv("WATCH_NAMESPACE")
	podName := os.Getenv("POD_NAME")

	opts.InitCbs.AddStartup(func(ctx context.Context) {
		sfReconciler.StartCache(ctx)
		if env.Config.Global != nil && env.Config.Global.Misc["enableLeaderElection"] == "on" {
			log.Infof("delete leader labels before working")
			deleteLeaderLabelUntilSucceed(env.K8SClient, podNs, podName)
		}
	})

	// build metric source
	source := metric.NewSource(pc)

	cache, err := controllers.NewCache(env)
	if err != nil {
		return fmt.Errorf("GetCacheFromServicefence occured err: %s", err)
	}
	source.Fullfill(cache)
	log.Debugf("GetCacheFromServicefence %+v", cache)

	// register svf reset
	handler := &server.Handler{
		HttpPathHandler: env.HttpPathHandler,
		Source:          source,
	}
	svfResetRegister(handler)

	var builder basecontroller.ObjectReconcilerBuilder

	// auto generate ServiceFence or not
	if m.config.AutoFence {
		builder = builder.Add(basecontroller.ObjectReconcileItem{
			Name:    "Namespace",
			ApiType: &corev1.Namespace{},
			R:       reconcile.Func(sfReconciler.ReconcileNamespace),
		}).Add(basecontroller.ObjectReconcileItem{
			Name:    "Service",
			ApiType: &corev1.Service{},
			R:       reconcile.Func(sfReconciler.ReconcileService),
		})
		// use FenceLabelKeyAlias ad switch to turn on/off workload fence
		if m.config.FenceLabelKeyAlias != "" {
			podController := sfReconciler.NewPodController(env.K8SClient, m.config.FenceLabelKeyAlias)
			le.AddOnStartedLeading(func(ctx context.Context) {
				go podController.Run(ctx.Done())
			})
		}
	}

	builder = builder.Add(basecontroller.ObjectReconcileItem{
		Name: "ServiceFence",
		R:    sfReconciler,
	}).Add(basecontroller.ObjectReconcileItem{
		Name: "VirtualService",
		R: &basecontroller.VirtualServiceReconciler{
			Env:    &env,
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		},
	})

	if err := builder.Build(mgr); err != nil {
		return fmt.Errorf("unable to create controller,%+v", err)
	}

	le.AddOnStartedLeading(func(ctx context.Context) {
		log.Infof("retrieve metric from svf status.metric")
		cache, err := controllers.NewCache(env)
		if err != nil {
			log.Warnf("GetCacheFromServicefence occured err in StartedLeading: %s", err)
			return
		}
		source.Fullfill(cache)
		log.Debugf("GetCacheFromServicefence is %+v", cache)
	})

	le.AddOnStartedLeading(func(ctx context.Context) {
		log.Infof("producers starts")
		metric.NewProducer(pc, source)
	})

	if m.config.AutoPort {
		le.AddOnStartedLeading(func(ctx context.Context) {
			sfReconciler.StartAutoPort(ctx)
		})
	}

	if env.Config.Metric != nil ||
		m.config.MetricSourceType == controllers.MetricSourceTypeAccesslog {
		le.AddOnStartedLeading(func(ctx context.Context) {
			go sfReconciler.WatchMetric(ctx)
		})
	} else {
		log.Warningf("watching metric is not running")
	}

	if env.Config.Global != nil && env.Config.Global.Misc["enableLeaderElection"] == "on" {

		log.Infof("add/delete leader label in StartedLeading/stoppedLeading")

		le.AddOnStartedLeading(func(ctx context.Context) {
			first := make(chan struct{}, 1)
			first <- struct{}{}
			var retry <-chan time.Time

			go func() {
				for {
					select {
					case <-ctx.Done():
						log.Infof("ctx is done, retrun")
						return
					case <-first:
					case <-retry:
						retry = nil
					}
					if err = addPodLabel(ctx, env.K8SClient, podNs, podName); err != nil {
						log.Errorf("add leader labels error %s, retry", err)
						retry = time.After(1 * time.Second)
					} else {
						log.Infof("add leader labels succeed")
						return
					}
				}
			}()
		})

		le.AddOnStoppedLeading(func() {
			go deleteLeaderLabelUntilSucceed(env.K8SClient, podNs, podName)
		})
	}

	le.AddOnStoppedLeading(sfReconciler.Clear)
	return nil
}

func svfResetRegister(handler *server.Handler) {
	handler.HandleFunc("/debug/svfReset", handler.SvfResetSetting)
}

func deleteLeaderLabelUntilSucceed(client *kubernetes.Clientset, podNs, podName string) {
	first := make(chan struct{}, 1)
	first <- struct{}{}
	var retry <-chan time.Time
	for {
		select {
		case <-first:
		case <-retry:
			retry = nil
		}

		if err := deletePodLabel(context.TODO(), client, podNs, podName); err != nil {
			log.Errorf("delete leader labels error %s", err)
			retry = time.After(1 * time.Second)
		} else {
			log.Infof("delete leader labels succeed")
			return
		}
	}
}

func addPodLabel(ctx context.Context, client *kubernetes.Clientset, podNs, podName string) error {
	po, err := getPod(ctx, client, podNs, podName)
	if err != nil {
		return err
	}

	po.Labels[modmodel.SlimeLeader] = "true"
	_, err = client.CoreV1().Pods(podNs).Update(ctx, po, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update pod namespace/name: %s/%s err %s", podNs, podName, err)
	}
	return nil
}

func deletePodLabel(ctx context.Context, client *kubernetes.Clientset, podNs, podName string) error {
	po, err := getPod(ctx, client, podNs, podName)
	if err != nil {
		return err
	}
	// if slime.io/leader not exist, skip
	if _, ok := po.Labels[modmodel.SlimeLeader]; !ok {
		log.Infof("label slime.io/leader is not found, skip")
		return nil
	}

	delete(po.Labels, modmodel.SlimeLeader)
	_, err = client.CoreV1().Pods(podNs).Update(ctx, po, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func getPod(ctx context.Context, client *kubernetes.Clientset, podNs, podName string) (*corev1.Pod, error) {
	pod, err := client.CoreV1().Pods(podNs).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			err = fmt.Errorf("pod %s/%s is not found", podNs, podName)
		} else {
			err = fmt.Errorf("get pod %s/%s err %s", podNs, podName, err)
		}
		return nil, err
	}
	return pod, nil
}
