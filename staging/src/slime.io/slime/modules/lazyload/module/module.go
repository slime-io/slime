package module

import (
	"os"

	"github.com/golang/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	istioapi "slime.io/slime/framework/apis"
	"slime.io/slime/framework/bootstrap"
	basecontroller "slime.io/slime/framework/controllers"
	"slime.io/slime/framework/model/module"
	lazyloadapiv1alpha1 "slime.io/slime/modules/lazyload/api/v1alpha1"
	"slime.io/slime/modules/lazyload/controllers"
	modmodel "slime.io/slime/modules/lazyload/model"
)

var log = modmodel.ModuleLog

type Module struct {
	config lazyloadapiv1alpha1.Fence
}

func (mo *Module) Kind() string {
	return modmodel.ModuleName
}

func (mo *Module) Config() proto.Message {
	return &mo.config
}

func (mo *Module) InitScheme(scheme *runtime.Scheme) error {
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

func (mo *Module) Clone() module.Module {
	ret := *mo
	return &ret
}

func (mo *Module) InitManager(mgr manager.Manager, env bootstrap.Environment, cbs module.InitCallbacks) error {
	cfg := &mo.config

	sfReconciler := controllers.NewReconciler(cfg, mgr, env)

	var builder basecontroller.ObjectReconcilerBuilder

	// auto generate ServiceFence or not
	if cfg == nil || cfg.AutoFence {
		builder = builder.Add(basecontroller.ObjectReconcileItem{
			Name:    "Namespace",
			ApiType: &corev1.Namespace{},
			R:       reconcile.Func(sfReconciler.ReconcileNamespace),
		}).Add(basecontroller.ObjectReconcileItem{
			Name:    "Service",
			ApiType: &corev1.Service{},
			R:       reconcile.Func(sfReconciler.ReconcileService),
		})
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
		log.Errorf("unable to create controller,%+v", err)
		os.Exit(1)
	}

	return nil
}
