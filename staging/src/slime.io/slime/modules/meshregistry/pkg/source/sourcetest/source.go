package sourcetest

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/libistio/pkg/config"
	"istio.io/libistio/pkg/config/event"
	"istio.io/libistio/pkg/config/resource"
	"istio.io/libistio/pkg/config/schema/collection"
	"istio.io/libistio/pkg/config/schema/collections"
	"istio.io/libistio/pkg/config/source/kube/file"
)

// simpleConfigStore is a simple in-memory config store that stores configs in a map.
// It is not thread-safe, and is only used for testing.
type simpleConfigStore struct {
	// snaps is a map of group version kind to a map of namespace/name to config.Config
	snaps map[config.GroupVersionKind]map[string]config.Config
}

// set sets the config for the given group version kind and namespace/name.
func (s *simpleConfigStore) set(cfg config.Config) {
	if s.snaps[cfg.GroupVersionKind] == nil {
		s.snaps[cfg.GroupVersionKind] = make(map[string]config.Config)
	}
	s.snaps[cfg.GroupVersionKind][cfg.Meta.Namespace+"/"+cfg.Meta.Name] = cfg
}

// remove removes the config for the given group version kind and namespace/name.
func (s *simpleConfigStore) remove(gvk config.GroupVersionKind, ns, name string) {
	delete(s.snaps[gvk], ns+"/"+name)
}

// listByGVK returns the configs for the given group version kind.
func (s *simpleConfigStore) listByGVK(gvk config.GroupVersionKind) map[string]config.Config {
	return s.snaps[gvk]
}

// clear clears the config store.
func (s *simpleConfigStore) clear() {
	s.snaps = make(map[config.GroupVersionKind]map[string]config.Config)
}

func (s *simpleConfigStore) Handle(e event.Event) {
	switch e.Kind {
	case event.Added, event.Updated:
		s.set(convertResourceToConfig(e.Source.GroupVersionKind(), e.Resource))
	case event.Deleted:
		ns, name := e.Resource.Metadata.FullName.Namespace.String(), e.Resource.Metadata.FullName.Name.String()
		s.remove(e.Source.GroupVersionKind(), ns, name)
	default:
		// do nothing
	}
}

type AssertEventHandler struct {
	schemas collection.Schemas

	expected       *simpleConfigStore
	expectedHelper *file.KubeSource

	got *simpleConfigStore
}

func NewAssertEventHandler() *AssertEventHandler {
	schemas := collection.SchemasFor(collections.ServiceEntry, collections.Sidecar)
	expected := &simpleConfigStore{
		snaps: map[config.GroupVersionKind]map[string]config.Config{},
	}
	helper := file.NewKubeSource(schemas)
	helper.Dispatch(expected)
	helper.Start()

	return &AssertEventHandler{
		schemas:        schemas,
		expected:       expected,
		expectedHelper: helper,
		got: &simpleConfigStore{
			snaps: map[config.GroupVersionKind]map[string]config.Config{},
		},
	}
}

func (h *AssertEventHandler) Handle(e event.Event) {
	h.got.Handle(e)
}

func (h *AssertEventHandler) LoadExpected(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return h.expectedHelper.ApplyContent(path, string(data))
}

func (h *AssertEventHandler) Reset() {
	h.expected.clear()
	h.expectedHelper.Clear()
	h.got.clear()
}

func (h *AssertEventHandler) Assert(t *testing.T) {
	for _, schema := range h.schemas.All() {
		expected := h.expected.listByGVK(schema.GroupVersionKind())
		got := h.got.listByGVK(schema.GroupVersionKind())
		assert.Equal(t, len(expected), len(got))
		for nn, expectCfg := range expected {
			gotCfg, exist := got[nn]
			if assert.Truef(t, exist, "expected config %s not found", nn) {
				assert.Equal(t, expectCfg.Meta.Labels, gotCfg.Meta.Labels)
				assert.Equal(t, expectCfg.Meta.Annotations, gotCfg.Meta.Annotations)
				expectSpecJson, _ := config.ToJSON(expectCfg.Spec)
				gotSpecJson, _ := config.ToJSON(gotCfg.Spec)
				assert.JSONEq(t, string(expectSpecJson), string(gotSpecJson))
			}
		}
	}
}

func (h *AssertEventHandler) DumpGot() {
	for _, schema := range h.schemas.All() {
		got := h.got.listByGVK(schema.GroupVersionKind())
		for _, cfg := range got {
			b, _ := json.Marshal(cfg)
			fmt.Println(string(b))
		}
	}
}

// convertResourceToConfig converts a resource.Instance to a config.Config.
func convertResourceToConfig(gvk config.GroupVersionKind, res *resource.Instance) config.Config {
	// Do not store the CreationTimestamp and ResourceVersion, because
	// we are only interested in the labels and the spec for testing.
	return config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk,
			Name:             res.Metadata.FullName.Name.String(),
			Namespace:        res.Metadata.FullName.Namespace.String(),
			Labels:           res.Metadata.Labels,
			Annotations:      res.Metadata.Annotations,
		},
		Spec: res.Message,
	}
}

// MockPollingClient is a mock polling client that loads services from a JSON file.
// It is used to test the polling mode source.
type MockPollingClient[T any] struct {
	err      error
	services []*T
	path     string
}

// Services returns the list of T and an error.
// If the error is not nil, it will return an empty list and the error.
// If the error is nil, and the services list is not nil, it will return the services list and nil.
// Otherwise, it will load the services from the JSON file and return them.
func (m *MockPollingClient[T]) Services() ([]*T, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.services != nil {
		return m.services, nil
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m.services, nil
}

func (m *MockPollingClient[T]) RegistryInfo() string {
	return "mock"
}

func (m *MockPollingClient[T]) Reset() {
	m.err = nil
	m.services = nil
	m.path = ""
}

func (m *MockPollingClient[T]) SetError(err error) {
	m.err = err
}

func (m *MockPollingClient[T]) SetServices(services []*T) {
	m.services = services
}

func (m *MockPollingClient[T]) SetPath(path string) {
	m.path = path
}

func (m *MockPollingClient[T]) Load(path string) error {
	if path != "" {
		m.SetPath(path)
	}
	return m.load()
}

func (m *MockPollingClient[T]) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	var apps []*T
	if err := json.Unmarshal(data, &apps); err != nil {
		return err
	}
	m.services = apps
	return nil
}
