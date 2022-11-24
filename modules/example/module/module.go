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
	"slime.io/slime/modules/example/api/config"
	v1alpha1 "slime.io/slime/modules/example/api/v1alpha1"
	"slime.io/slime/modules/example/controllers"
	"slime.io/slime/modules/example/model"
)

var log = model.ModuleLog

type Module struct {
	config config.Example
}

func (mo *Module) Kind() string {
	return model.ModuleName
}

func (mo *Module) Config() proto.Message {
	return &mo.config
}

func (mo *Module) InitScheme(scheme *runtime.Scheme) error {
	for _, f := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		v1alpha1.AddToScheme,
		istionetworkingapi.AddToScheme,
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

	var err error
	if err = (&controllers.ExampleReconciler{
		Cfg: cfg, Env: &env,
	}).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create example controller, %+v", err)
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
