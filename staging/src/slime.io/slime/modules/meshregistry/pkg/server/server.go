/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package server

import (
	"google.golang.org/grpc"

	slimebootstrap "slime.io/slime/framework/bootstrap"
	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "server")

type Server struct {
	p *Processing
}

type Args struct {
	SlimeEnv     slimebootstrap.Environment
	RegistryArgs *bootstrap.RegistryArgs
	// AddOnRegArgs should be called only in `new` stage. NOT IN `RUN` stage
	AddOnRegArgs func(onConfig func(args *bootstrap.RegistryArgs))
}

func NewServer(args *Args) (*Server, error) {
	grpc.EnableTracing = args.RegistryArgs.EnableGRPCTracing
	proc := NewProcessing(args)
	return &Server{
		p: proc,
	}, nil
}

func (s *Server) Run(stop <-chan struct{}) error {
	if err := s.p.Start(); err != nil {
		return err
	}
	<-stop
	s.p.Stop()
	return nil
}
