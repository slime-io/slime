package resource

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

type ValidateFunc func(name, namespace string, config proto.Message) error

// SubBuilder for a Schema.
type SubBuilder struct {
	// ClusterScoped is true for resource in cluster-level.
	ClusterScoped bool

	// Kind is the config proto type.
	Kind string

	// Plural is the type in plural.
	Plural string

	// Group is the config proto group.
	Group string

	// Version is the config proto version.
	Version string

	// Proto refers to the protobuf message type name corresponding to the type
	Proto string

	// ProtoPackage refers to the name of golang package for the protobuf message.
	ProtoPackage string

	// ValidateProto performs validation on protobuf messages based on this schema.
	ValidateProto ValidateFunc
}

// Build a Schema instance.
func (b SubBuilder) Build() (GroupVersionKind, error) {
	s := b.BuildNoValidate()

	return s, nil
}

// MustBuild calls Build and panics if it fails.
func (b SubBuilder) MustBuild() GroupVersionKind {
	s, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("MustBuild: %v", err))
	}
	return s
}

// BuildNoValidate builds the Schema without checking the fields.
func (b SubBuilder) BuildNoValidate() GroupVersionKind {
	return GroupVersionKind{
		Group:   b.Group,
		Version: b.Version,
		Kind:    b.Kind,
	}
}
