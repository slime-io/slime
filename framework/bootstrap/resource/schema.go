package resource

import "fmt"

type Schemas struct {
	byCollection map[string]GroupVersionKind
	byAddOrder   []GroupVersionKind
}

// FindByGroupVersionKind Get
func (s Schemas) FindByGroupVersionKind(gvk GroupVersionKind) (GroupVersionKind, bool) {
	for _, rs := range s.byAddOrder {
		if rs.String() == gvk.String() {
			return rs, true
		}
	}

	return GroupVersionKind{}, false
}

// All List
func (s Schemas) All() []GroupVersionKind {
	return append(make([]GroupVersionKind, 0, len(s.byAddOrder)), s.byAddOrder...)
}

// Add creates a copy of this Schemas with the given schemas added.
func (s Schemas) Add(toAdd ...GroupVersionKind) Schemas {
	b := NewSchemasBuilder()

	for _, gvk := range s.byAddOrder {
		b.MustAdd(gvk)
	}

	for _, gvk := range toAdd {
		b.MustAdd(gvk)
	}

	return b.Build()
}

// Remove creates a copy of this Schemas with the given schemas removed.
func (s Schemas) Remove(toRemove ...GroupVersionKind) Schemas {
	b := NewSchemasBuilder()

	for _, gvk := range s.byAddOrder {
		shouldAdd := true
		for _, r := range toRemove {
			if r.String() == gvk.String() {
				shouldAdd = false
				break
			}
		}
		if shouldAdd {
			b.MustAdd(gvk)
		}
	}

	return b.Build()
}

// SchemasBuilder is a builder for the schemas type.
type SchemasBuilder struct {
	schemas Schemas
}

// NewSchemasBuilder returns a new instance of SchemasBuilder.
func NewSchemasBuilder() *SchemasBuilder {
	s := Schemas{
		byCollection: make(map[string]GroupVersionKind),
	}

	return &SchemasBuilder{
		schemas: s,
	}
}

func (b *SchemasBuilder) Add(gvk GroupVersionKind) error {
	if _, found := b.schemas.byCollection[gvk.String()]; found {
		return fmt.Errorf("collection already exists: %v", gvk.String())
	}

	b.schemas.byCollection[gvk.String()] = gvk
	b.schemas.byAddOrder = append(b.schemas.byAddOrder, gvk)
	return nil
}

func (b *SchemasBuilder) MustAdd(gvk GroupVersionKind) *SchemasBuilder {
	if err := b.Add(gvk); err != nil {
		panic(fmt.Sprintf("SchemasBuilder.MustAdd: %v", err))
	}
	return b
}

func (b *SchemasBuilder) Build() Schemas {
	s := b.schemas

	// Avoid modify after Build.
	b.schemas = Schemas{}

	return s
}
