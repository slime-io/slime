package module

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/modules/plugin/api/config"
	pluginapiv1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1"
	"slime.io/slime/modules/plugin/controllers"
	"slime.io/slime/modules/plugin/model"
)

var log = model.ModuleLog

type Module struct {
	config config.PluginModule
}

func (m *Module) Kind() string {
	return model.ModuleName
}

func (m *Module) Config() proto.Message {
	return &m.config
}

func (m *Module) InitScheme(scheme *runtime.Scheme) error {
	for _, f := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		pluginapiv1alpha1.AddToScheme,
		networkingv1alpha3.AddToScheme,
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
	cfg := &m.config
	env := opts.Env
	mgr := opts.Manager

	var err error
	pmr := controllers.NewPluginManagerReconciler(env, mgr.GetClient(), mgr.GetScheme(), cfg)
	if opts.LeaderElectionCbs != nil {
		opts.LeaderElectionCbs.AddOnStartedLeading(pmr.OnStartLeading)
	}
	if err = pmr.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create pluginManager controller, %+v", err)
	}
	if err = (&controllers.EnvoyPluginReconciler{
		Env:    &env,
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Cfg:    cfg,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create EnvoyPlugin controller, %+v", err)
	}

	return nil
}
