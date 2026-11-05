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

// ScopedStore is a Store that can also answer a narrowed list query. FindWhere
// matches the non-zero fields of match conjunctively; a zero field means "don't
// care", so "ParentID is empty" must be filtered in memory over the narrowed set.
type ScopedStore[T any, K comparable] interface {
	Store[T, K]
	FindWhere(
		ctx context.Context,
		match T,
	) ([]T, error)
}
