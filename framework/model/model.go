package model

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	LogFieldKeyModule   = "module"
	LogFieldKeyPkg      = "pkg"
	LogFieldKeyFunction = "func"
	LogFieldKeyResource = "resource"
)

type ModuleEvent struct {
	NN       types.NamespacedName
	Material map[string]string
}

type WatcherEvent struct {
	GVK schema.GroupVersionKind
	NN  types.NamespacedName
}
