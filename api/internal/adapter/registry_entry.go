package adapter

import (
	"container/list"
	"sync"
)

type registryEntry[T any] struct {
	key      string
	value    T
	opened   bool
	refcount int
	elem     *list.Element
	openMu   sync.Mutex
}
