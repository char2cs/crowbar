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
