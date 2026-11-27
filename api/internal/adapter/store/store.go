package store

import "context"

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

type ScopedStore[T any, K comparable] interface {
	Store[T, K]
	FindWhere(
		ctx context.Context,
		match T,
	) ([]T, error)
}
