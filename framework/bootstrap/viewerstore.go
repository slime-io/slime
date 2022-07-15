package bootstrap

import (
	"github.com/hashicorp/go-multierror"
	"slime.io/slime/framework/bootstrap/resource"
	"slime.io/slime/framework/bootstrap/viewstore"
)

// xdsViewerStore converts multi xds config store to aggregated viewer store
// only supports reading cache content
type viewerStore struct {
	// schemas is the unified
	schemas resource.Schemas
	// stores is a mapping from config type to a store
	stores map[resource.GroupVersionKind][]ConfigStore
}

// makeXdsViewerStore creates aggregated viewer store from several config stores
func makeViewerStore(mcs []*monitorController) (viewstore.ViewerStore, error) {
	union := resource.NewSchemasBuilder()
	storeTypes := make(map[resource.GroupVersionKind][]ConfigStore)
	for _, mc := range mcs {
		for _, gvk := range mc.Schemas().All() {
			if len(storeTypes[gvk]) == 0 {
				if err := union.Add(gvk); err != nil {
					return nil, err
				}
			}
			storeTypes[gvk] = append(storeTypes[gvk], mc)
		}
	}

	schemas := union.Build()

	return &viewerStore{
		schemas: schemas,
		stores:  storeTypes,
	}, nil
}

func (vs *viewerStore) Schemas() resource.Schemas {
	return vs.schemas
}

// Get the first config found in the stores.
func (vs *viewerStore) Get(gvk resource.GroupVersionKind, name, namespace string) *resource.Config {
	for _, store := range vs.stores[gvk] {
		config := store.Get(gvk, name, namespace)
		if config != nil {
			return config
		}
	}

	return nil
}

// List all configs in the stores.
func (vs *viewerStore) List(gvk resource.GroupVersionKind, namespace string) ([]resource.Config, error) {
	if len(vs.stores[gvk]) == 0 {
		return nil, nil
	}
	var errs *multierror.Error
	var configs []resource.Config
	// Used to remove duplicated config
	configMap := make(map[string]struct{})

	for _, store := range vs.stores[gvk] {
		storeConfigs, err := store.List(gvk, namespace)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
		for _, config := range storeConfigs {
			key := config.GroupVersionKind.Kind + config.Namespace + config.Name
			if _, exist := configMap[key]; exist {
				continue
			}
			configs = append(configs, config)
			configMap[key] = struct{}{}
		}
	}
	return configs, errs.ErrorOrNil()
}
