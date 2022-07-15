package resource

import "fmt"

// Builder Config for the creation of a Schema
type Builder struct {
	Name         string
	VariableName string
	Disabled     bool
	Resource     GroupVersionKind
}

// Build a Schema instance.
func (b Builder) Build() (GroupVersionKind, error) {
	if b.Resource.Group == "" && b.Resource.Version == "" && b.Resource.Kind == "" {
		return GroupVersionKind{}, fmt.Errorf("collection %s: resource must be non-nil", b.Name)
	}

	return b.Resource, nil
}

// MustBuild calls Build and panics if it fails.
func (b Builder) MustBuild() GroupVersionKind {
	s, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("MustBuild: %v", err))
	}

	return s
}
