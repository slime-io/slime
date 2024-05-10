package source

import (
	"net/http"
	"strings"
	"time"

	resource2 "istio.io/istio-mcp/pkg/config/schema/resource"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collections"

	frameworkmodel "slime.io/slime/framework/model"
	"slime.io/slime/modules/meshregistry/model"
	"slime.io/slime/modules/meshregistry/pkg/bootstrap"
	"slime.io/slime/modules/meshregistry/pkg/features"
)

const (
	ModePolling  = "polling"
	ModeWatching = "watching"

	ProjectCode = "projectCode"

	CacheRegistryInfoQueryKey = "registries"
)

var log = model.ModuleLog.WithField(frameworkmodel.LogFieldKeyPkg, "source")

type RegistryInfo struct {
	RegistryID string   `json:"registry_id,omitempty"`
	Addresses  []string `json:"addresses,omitempty"`
}

// RegistrySourceInitlizer is the initlizer of registry source,
// it will be called when the registry source is needed created.
type RegistrySourceInitlizer func(
	// args is the args of the meshregistry, it is used to initialize the registry source.
	args *bootstrap.RegistryArgs,
	// sourceReadyCallback is the callback function that will be called by the registry source when it is ready.
	sourceReadyCallback func(string),
	// addOnReArgs is used to register a callback function that will
	// be called when the args of the meshregistry is changed.
	addOnReArgs func(onReArgsCallback func(args *bootstrap.RegistryArgs)),
) (
	// source is the event.Source implementation of the registry source.
	source event.Source,
	// debugHandler is the map of http.HandlerFunc that will be used to handle the debug requests.
	debugHandler map[string]http.HandlerFunc,
	// cacheCluster is the flag that indicates whether the meshregistry should cache the cluster info.
	cacheCluster bool,
	// skip is the flag that indicates whether the meshregistry should
	// skip the initialization process for the registry source.
	skip bool,
	// err is the error that may occur during the initialization process.
	err error,
)

var registrySources map[string]RegistrySourceInitlizer

func RegistrySources() map[string]RegistrySourceInitlizer {
	return registrySources
}

// RegisterSourceInitlizer registers the initlizer of registry source.
// It is not concurrent safe, and recommend to be called in init function.
func RegisterSourceInitlizer(sourceName string, initlizer RegistrySourceInitlizer) {
	if registrySources == nil {
		registrySources = make(map[string]RegistrySourceInitlizer)
	}
	registrySources[sourceName] = initlizer
}

func GenVersion() resource.Version {
	return resource.Version(time.Now().String())
}

func IsInternalVersion(ver resource.Version) bool {
	return strings.Contains(string(ver), "-")
}

func IsInternalResource(gvk resource2.GroupVersionKind) bool {
	return gvk.Kind == collections.ServiceEntry.Kind() || gvk.Kind == collections.Sidecar.Kind()
}

func FillRevision(meta *resource.Metadata) bool {
	if features.IstioRevision != "" {
		exist, ok := meta.Labels[frameworkmodel.IstioRevLabel]
		meta.Labels[frameworkmodel.IstioRevLabel] = features.IstioRevision
		return !ok || exist != features.IstioRevision
	}

	return false
}

func GetProjectCodeArr[T, I any](t T, instances func(T) []I, meta func(I) map[string]string) []string {
	projectCodes := make([]string, 0)
	projectCodeMap := make(map[string]struct{})
	for _, instance := range instances(t) {
		if v, ok := meta(instance)[ProjectCode]; ok {
			if _, ok := projectCodeMap[v]; !ok {
				projectCodes = append(projectCodes, v)
				projectCodeMap[v] = struct{}{}
			}
		}
	}

	return projectCodes
}

func BuildInstanceMetaModifier(rl *bootstrap.InstanceMetaRelabel) func(*map[string]string) {
	if rl == nil || len(rl.Items) == 0 {
		return nil
	}
	relabelFuncs := make([]func(*map[string]string), 0, len(rl.Items))
	for _, relabel := range rl.Items {
		f := func(item *bootstrap.InstanceMetaRelabelItem) func(*map[string]string) {
			return func(mPtr *map[string]string) {
				m := *mPtr
				if item.Key == "" || item.TargetKey == "" {
					return
				}
				if len(m) == 0 && item.CreatedWithValue == "" {
					return
				}
				if m == nil {
					m = make(map[string]string)
				}
				v, exist := m[item.Key]
				if !exist && item.CreatedWithValue != "" {
					v, exist = item.CreatedWithValue, true
				}
				if !exist {
					return
				}
				if nv, ok := item.ValuesMapping[v]; ok {
					v = nv
				}

				if _, exist := m[item.TargetKey]; !exist || item.Overwrite {
					m[item.TargetKey] = v
				}
				*mPtr = m
			}
		}(relabel)
		relabelFuncs = append(relabelFuncs, f)
	}
	return func(mPtr *map[string]string) {
		for _, f := range relabelFuncs {
			f(mPtr)
		}
	}
}
