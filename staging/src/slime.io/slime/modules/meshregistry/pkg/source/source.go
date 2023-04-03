package source

import (
	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/features"
	"strings"
	"time"

	resource2 "istio.io/istio-mcp/pkg/config/schema/resource"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"
)

var (
	kindServiceEntry = collections.K8SNetworkingIstioIoV1Alpha3Serviceentries.Resource().Kind()
	kindSidecar      = collections.K8SNetworkingIstioIoV1Alpha3Sidecars.Resource().Kind()
	IstioRevisionKey = "istio.io/rev"

	log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "source")
)

func GenVersion(sch collection.Schema) resource.Version {
	return resource.Version(time.Now().String())
}

func IsInternalVersion(ver resource.Version) bool {
	return strings.Contains(string(ver), "-")
}

func IsInternalResource(gvk resource2.GroupVersionKind) bool {
	return gvk.Kind == kindServiceEntry || gvk.Kind == kindSidecar
}

func FillRevision(meta resource.Metadata) {
	if features.IstioRevision != "" {
		meta.Labels[IstioRevisionKey] = features.IstioRevision
	}
}
