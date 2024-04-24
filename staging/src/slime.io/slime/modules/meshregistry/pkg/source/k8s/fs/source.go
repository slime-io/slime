package fs

import (
	"net/http"

	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/source/kube/file"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/source"
	"slime.io/slime/modules/meshregistry/pkg/source/k8s"
)

const (
	SourceName = "kubefs"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "zk")

func init() {
	source.RegisterSourceInitlizer(SourceName, source.RegistrySourceInitlizer(New))
}

type Source struct {
	event.Source

	initedCallback func(string)
}

func New(
	moduleArgs *bootstrap.RegistryArgs,
	readyCallback func(string),
	_ func(func(*bootstrap.RegistryArgs)),
) (event.Source, map[string]http.HandlerFunc, bool, bool, error) {
	args := moduleArgs.K8SSource
	if !args.Enabled || !args.EnableConfigFile || args.ConfigPath == "" {
		return nil, nil, false, true, nil
	}
	collections := append([]string(nil), args.Collections...)
	collections = append(collections, moduleArgs.Snapshots...)
	excludeKinds := append([]string(nil), args.ExcludedResourceKinds...)
	excludeKinds = append(excludeKinds, moduleArgs.ExcludedResourceKinds...)
	schemas := k8s.BuildKubeSourceSchemas(collections, excludeKinds)
	kubeFsSrc, err := file.New(args.ConfigPath, schemas, args.WatchConfigFiles)
	if err != nil {
		log.Errorf("Failed to create kube fs source: %v", err)
		return nil, nil, false, false, err
	}
	src := &Source{
		Source:         kubeFsSrc,
		initedCallback: readyCallback,
	}

	return src, nil, false, false, nil
}

func (s *Source) Start() {
	// we can't control the concurrency of the kube fs source, so we need to start it in a goroutine, then mark it ready
	go s.Source.Start()
	s.initedCallback(SourceName)
}
