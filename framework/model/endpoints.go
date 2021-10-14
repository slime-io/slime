package model

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type Endpoints struct {
	Location types.NamespacedName
	Info     map[string]string
	Lock     sync.RWMutex
}
