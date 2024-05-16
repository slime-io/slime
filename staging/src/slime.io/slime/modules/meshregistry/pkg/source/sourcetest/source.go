package sourcetest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-zookeeper/zk"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
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

// AssertEventHandler is a helper for testing `event.Source`.
// It loads the expected configs from a yaml format file, and
// processes events from `event.Source` to build the got configs.
type AssertEventHandler struct {
	schemas collection.Schemas

	expected       *simpleConfigStore
	expectedHelper *file.KubeSource

	got *simpleConfigStore
}

// NewAssertEventHandler creates a new AssertEventHandler.
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

// Handle handles an event.Event. Implement `event.Handler`.
func (h *AssertEventHandler) Handle(e event.Event) {
	h.got.Handle(e)
}

// LoadExpected loads the expected configs from a yaml format file.
func (h *AssertEventHandler) LoadExpected(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return h.expectedHelper.ApplyContent(path, string(data))
}

// Reset clears both the expected and got configs.
func (h *AssertEventHandler) Reset() {
	h.expected.clear()
	h.expectedHelper.Clear()
	h.got.clear()
}

// Assert asserts that the expected and got configs are equal.
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

// ZkNode is a node in the Zookeeper tree.
type ZkNode struct {
	// FullPath is the full path of the node, including the leading slash.
	FullPath string `json:"fullPath,omitempty" yaml:"fullPath"`
	// Data is the data of the node. Optional.
	Data string `json:"data,omitempty" yaml:"data"`
	// Children is the children of the node. Optional.
	Children map[string]*ZkNode `json:"children,omitempty" yaml:"children"`
}

func (n *ZkNode) GetChildren(path string) []string {
	if n.FullPath == path {
		children := make([]string, 0, len(n.Children))
		for _, child := range n.Children {
			children = append(children, filepath.Base(child.FullPath))
		}
		return children
	}
	if strings.HasPrefix(path, n.FullPath) {
		for _, child := range n.Children {
			if !strings.HasPrefix(path, child.FullPath) {
				continue
			}
			children := child.GetChildren(path)
			if len(children) > 0 {
				return children
			}
		}
	}
	return nil
}

type MockZookeeperClient struct {
	err  error
	root *ZkNode
	path string
}

func NewMockZookeeperClient() *MockZookeeperClient {
	return &MockZookeeperClient{}
}

func (m *MockZookeeperClient) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root ZkNode
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	m.root = &root
	return nil
}

func (m *MockZookeeperClient) Children(path string) ([]string, error) {
	children := m.root.GetChildren(path)
	if children == nil {
		return nil, fmt.Errorf("children of path %s not found", path)
	}
	return children, nil
}

func (m *MockZookeeperClient) ChildrenW(_ string) ([]string, <-chan zk.Event, error) {
	panic("implement me")
}

func (m *MockZookeeperClient) Reset() {
	m.root = nil
}

func (m *MockZookeeperClient) SetError(err error) {
	m.err = err
}

func (m *MockZookeeperClient) SetRoot(root *ZkNode) {
	m.root = root
}

func (m *MockZookeeperClient) SetPath(path string) {
	m.path = path
}
