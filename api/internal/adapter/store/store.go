// Package store provides generic, swappable storage implementations.
// Any Crowbar component can depend on these interfaces and swap implementations freely.
package store

import "context"

// Store is a generic CRUD repository over a GORM-mapped entity T keyed by K.
type Store[T any, K comparable] interface {
	Save(
		ctx context.Context,
		item T,
	) error
	Delete(
		ctx context.Context,
		id K,
	) error
	FindByKey(
		ctx context.Context,
		id K,
	) (*T, error)
	FindAll(
		ctx context.Context,
	) ([]T, error)
}

// ScopedStore is a Store that can additionally answer a narrowed list query, so
// a caller that needs one parent's rows never materialises the whole table.
//
// FindWhere matches on the NON-ZERO fields of match, conjunctively. A zero field
// is not a filter — it is "don't care" — so this can express "every folder under
// project P and repo R" but never "every folder whose ParentID is empty"; that
// distinction is drawn in memory over an already-narrowed set.
type ScopedStore[T any, K comparable] interface {
	Store[T, K]
	FindWhere(
		ctx context.Context,
		match T,
	) ([]T, error)
}
