package bootstrap

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"slime.io/slime/framework/bootstrap/resource"
)

var (
	errNotFound = errors.New("item not found")
)

const ResourceVersion string = "ResourceVersion"

type ConfigStore interface {
	// Schemas exposes the configuration type schema known by the config store.
	Schemas() resource.Schemas
	// Get retrieves a configuration element by a type and a key
	Get(gvk resource.GroupVersionKind, name, namespace string) *resource.Config
	// List returns objects by type and namespace.
	// Use "" for the namespace to list across namespaces.
	List(typ resource.GroupVersionKind, namespace string) ([]resource.Config, error)
	// Create adds a new configuration object to the store. If an object with the
	// same name and namespace for ProtoMessagethe type already exists, the operation fails
	// with no side effects.
	Create(config resource.Config) (revision string, err error)
	// Update modifies an existing configuration object in the store.  Update
	// requires that the object has been created.  Resource version prevents
	// overriding a value that has been changed between prior _Get_ and _Put_
	// operation to achieve optimistic concurrency. This method returns a new
	// revision if the operation succeeds.
	Update(config resource.Config) (newRevision string, err error)
	// Delete removes an object from the store by key
	Delete(typ resource.GroupVersionKind, name, namespace string) error
}

// configStore inits cache store for single xds server.
// supports reading and writing cache content according to latest xds data
type configStore struct {
	schemas resource.Schemas
	data    map[resource.GroupVersionKind]map[string]*sync.Map
	mutex   sync.RWMutex
}

func makeConfigStore(schemas resource.Schemas) ConfigStore {
	out := configStore{
		schemas: schemas,
		data:    make(map[resource.GroupVersionKind]map[string]*sync.Map),
	}
	for _, gvk := range schemas.All() {
		out.data[gvk] = make(map[string]*sync.Map)
	}
	return &out
}

func (cr *configStore) Schemas() resource.Schemas {
	return cr.schemas
}

func (cr *configStore) Get(gvk resource.GroupVersionKind, name, namespace string) *resource.Config {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()
	_, ok := cr.data[gvk]
	if !ok {
		return nil
	}

	ns, exists := cr.data[gvk][namespace]
	if !exists {
		return nil
	}

	out, exists := ns.Load(name)
	if !exists {
		return nil
	}
	config := out.(resource.Config)

	return &config
}

func (cr *configStore) List(kind resource.GroupVersionKind, namespace string) ([]resource.Config, error) {
	cr.mutex.RLock()
	defer cr.mutex.RUnlock()
	data := cr.data
	if kind != resource.AllGroupVersionKind {
		kindData, exists := cr.data[kind]
		if !exists {
			return nil, nil
		}
		data = map[resource.GroupVersionKind]map[string]*sync.Map{kind: kindData}
	}

	out := make([]resource.Config, 0, len(cr.data[kind]))
	for _, kindData := range data {
		if namespace != "" {
			nsData, exist := kindData[namespace]
			if !exist {
				continue
			}
			kindData = map[string]*sync.Map{namespace: nsData}
		}

		for _, nsData := range kindData {
			nsData.Range(func(key, value interface{}) bool {
				out = append(out, value.(resource.Config))
				return true
			})
		}
	}

	return out, nil
}

func (cr *configStore) Create(config resource.Config) (string, error) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()
	gvk := config.GroupVersionKind
	_, ok := cr.schemas.FindByGroupVersionKind(gvk)
	if !ok {
		return "", fmt.Errorf("unknown type %+v", gvk)
	}
	ns, exists := cr.data[gvk][config.Namespace]
	if !exists {
		ns = new(sync.Map)
		cr.data[gvk][config.Namespace] = ns
	}

	_, exists = ns.Load(config.Name)

	if !exists {
		tnow := time.Now()
		if config.Annotations != nil && config.Annotations[ResourceVersion] != "" {
			config.ResourceVersion = config.Annotations[ResourceVersion]
			delete(config.Annotations, ResourceVersion)
		} else {
			config.ResourceVersion = tnow.String()
		}

		// Set the creation timestamp, if not provided.
		if config.CreationTimestamp.IsZero() {
			config.CreationTimestamp = tnow
		}

		ns.Store(config.Name, config)
		return config.ResourceVersion, nil
	}
	return "", errors.New(fmt.Sprintf("%s %s/%s already exists", config.GroupVersionKind.String(), config.Namespace, config.Name))
}

func (cr *configStore) Update(config resource.Config) (string, error) {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()
	gvk := config.GroupVersionKind
	_, ok := cr.schemas.FindByGroupVersionKind(gvk)
	if !ok {
		return "", errors.New("unknown type")
	}

	ns, exists := cr.data[gvk][config.Namespace]
	if !exists {
		return "", errNotFound
	}

	existing, exists := ns.Load(config.Name)
	if !exists {
		return "", errNotFound
	}

	if config.Annotations != nil && config.Annotations[ResourceVersion] != "" {
		config.ResourceVersion = config.Annotations[ResourceVersion]
		if config.ResourceVersion == (existing.(resource.Config)).ResourceVersion {
			return config.ResourceVersion, nil
		}
		delete(config.Annotations, ResourceVersion)
	} else {
		config.ResourceVersion = time.Now().String()
	}

	ns.Store(config.Name, config)
	return config.ResourceVersion, nil
}

func (cr *configStore) Delete(gvk resource.GroupVersionKind, name, namespace string) error {
	cr.mutex.Lock()
	defer cr.mutex.Unlock()
	data, ok := cr.data[gvk]
	if !ok {
		return errors.New("unknown type")
	}
	ns, exists := data[namespace]
	if !exists {
		return errNotFound
	}

	_, exists = ns.Load(name)
	if !exists {
		return errNotFound
	}

	ns.Delete(name)
	return nil
}
