/*
* @Author: yangdihang
* @Date: 2020/8/31
 */

package server

import (
	"os"
	slimebootstrap "slime.io/slime/framework/bootstrap"

	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
)

type Server struct {
	p *Processing
}

type Args struct {
	SlimeEnv     slimebootstrap.Environment
	RegistryArgs *bootstrap.RegistryArgs
}

func NewServer(args *Args) (*Server, error) {
	os.Setenv("istio-revision", args.RegistryArgs.Revision)
	os.Setenv("rev-crds", args.RegistryArgs.RevCrds)
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
