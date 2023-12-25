package module

import (
	"google.golang.org/protobuf/proto"
	networkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"slime.io/slime/framework/model/module"
	"slime.io/slime/modules/example/api/config"
	v1alpha1 "slime.io/slime/modules/example/api/v1alpha1"
	"slime.io/slime/modules/example/controllers"
	"slime.io/slime/modules/example/model"
)

var log = model.ModuleLog

type Module struct {
	config config.Example
}

func (mo *Module) Setup(opts module.ModuleOptions) error {
	env := opts.Env
	mgr := opts.Manager
	cfg := &mo.config

	var err error
	if err = (&controllers.ExampleReconciler{
		Cfg: cfg, Env: &env, Scheme: mgr.GetScheme(), Client: mgr.GetClient(),
	}).SetupWithManager(mgr); err != nil {
		log.Errorf("unable to create example controller, %+v", err)
		return err
	}
	return nil
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
		networkingv1alpha3.AddToScheme,
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
