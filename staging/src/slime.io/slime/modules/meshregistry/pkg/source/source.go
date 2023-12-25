package source

import (
	"strings"
	"time"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/features"

	resource2 "istio.io/istio-mcp/pkg/config/schema/resource"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"
)

var (
	kindServiceEntry = collections.ServiceEntry.Kind()
	kindSidecar      = collections.Sidecar.Kind()
	IstioRevisionKey = "istio.io/rev"

	log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "source")

	ServiceEntry = collection.Builder{
		Resource: collections.ServiceEntry,
	}.MustBuild()
	Sidecar = collection.Builder{
		Resource: collections.Sidecar,
	}.MustBuild()
)

func GenVersion() resource.Version {
	return resource.Version(time.Now().String())
}

func IsInternalVersion(ver resource.Version) bool {
	return strings.Contains(string(ver), "-")
}

func IsInternalResource(gvk resource2.GroupVersionKind) bool {
	return gvk.Kind == kindServiceEntry || gvk.Kind == kindSidecar
}

func FillRevision(meta resource.Metadata) bool {
	if features.IstioRevision != "" {
		exist, ok := meta.Labels[IstioRevisionKey]
		meta.Labels[IstioRevisionKey] = features.IstioRevision
		return !ok || exist != features.IstioRevision
	}

	return false
}
