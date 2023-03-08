package module

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	istionetworkingapi "slime.io/slime/framework/apis/networking/v1alpha3"
	"slime.io/slime/framework/model/module"
	"slime.io/slime/framework/util"
	"slime.io/slime/modules/meshregistry/model"
	meshregbootstrap "slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/server"
)

var log = model.ModuleLog

type Module struct {
	config util.AnyMessage
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

func (m *Module) Setup(opts module.ModuleOptions) error {
	regArgs := meshregbootstrap.NewRegistryArgs()

	type legacyWrapper struct {
		Legacy json.RawMessage `json:"LEGACY"`
	}

	if rawJson := m.config.RawJson; rawJson != nil {
		var lw legacyWrapper
		if err := json.Unmarshal(rawJson, &lw); err != nil {
			log.Errorf("invalid raw json: %s", string(rawJson))
			return err
		}

		if lw.Legacy != nil {
			if err := json.Unmarshal(lw.Legacy, regArgs); err != nil {
				log.Errorf("invalid raw json: %s", string(rawJson))
				return err
			}
		}
	}

	if regArgs == nil {
		return fmt.Errorf("nil registry args")
	}
	if err := regArgs.Validate(); err != nil {
		return fmt.Errorf("invalid args for meshregsitry: %w", err)
	}
	bs, err := json.MarshalIndent(regArgs, "", "  ")
	log.Infof("inuse registry args: %s, err %v", string(bs), err)

	cbs := opts.InitCbs
	cbs.AddStartup(func(ctx context.Context) {
		go func() {
			// Create the server for the discovery service.
			registryServer, err := server.NewServer(&server.Args{
				SlimeEnv:     opts.Env,
				RegistryArgs: regArgs,
			})
			if err != nil {
				log.Errorf("failed to create discovery service: %v", err)
				return
			}

			// Start the server
			if err := registryServer.Run(ctx.Done()); err != nil {
				log.Errorf("failed to start discovery service: %v", err)
				return
			}
		}()
	})

	return nil
}
