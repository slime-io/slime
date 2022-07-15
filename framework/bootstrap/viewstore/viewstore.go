package viewstore

import "slime.io/slime/framework/bootstrap/resource"

type ViewerStore interface {
	// Schemas exposes the configuration type schema known by the config store.
	Schemas() resource.Schemas
	// Get retrieves a configuration element by a type and a key
	Get(gvk resource.GroupVersionKind, name, namespace string) *resource.Config
	// List returns objects by type and namespace.
	// Use "" for the namespace to list across namespaces.
	List(gvk resource.GroupVersionKind, namespace string) ([]resource.Config, error)
}
