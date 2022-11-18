package module

import (
	"os"

	"github.com/golang/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	istionetworkingapi "slime.io/slime/framework/apis/networking/v1alpha3"
	"slime.io/slime/framework/bootstrap"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/framework/model/pkg/leaderelection"
	pluginapiv1alpha1 "slime.io/slime/modules/plugin/api/v1alpha1"
	"slime.io/slime/modules/plugin/controllers"
	"slime.io/slime/modules/plugin/model"
)

var log = model.ModuleLog

type Module struct {
	config pluginapiv1alpha1.PluginModule
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
		istionetworkingapi.AddToScheme,
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

func (m *Module) InitManager(mgr manager.Manager, env bootstrap.Environment, cbs module.InitCallbacks) error {
	cfg := &m.config

	_ = cfg // unused until now

	var err error
	if err = controllers.NewPluginManagerReconciler(env, mgr.GetClient(), mgr.GetScheme()).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create pluginManager controller, %+v", err)
		os.Exit(1)
	}
	if err = (&controllers.EnvoyPluginReconciler{
		Env:    &env,
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create EnvoyPlugin controller, %+v", err)
		os.Exit(1)
	}

	return nil
}

func (m *Module) Init(env bootstrap.Environment) error {
	return nil
}

func (m *Module) SetupWithInitCallbacks(cbs module.InitCallbacks) error {
	return nil
}

func (m *Module) SetupWithManager(mgr manager.Manager) error {
	return nil
}

func (m *Module) SetupWithLeaderElection(le leaderelection.LeaderCallbacks) error {
	return nil
}
