package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Store interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.Workspace, error)
	Drop(
		ctx context.Context,
		id string,
	) error
	AwaitTombstone(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	ListOrRebuild(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

type service struct {
	store *projections.Store
	es    asynxModels.Store
	ax    asynx.Asynx[domain.Workspace]
}

func New(
	st *projections.Store,
	es asynxModels.Store,
	ax asynx.Asynx[domain.Workspace],
) Store {
	return &service{store: st, es: es, ax: ax}
}

func RegisterHub[F any](
	s Store,
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) error {
	svc, ok := s.(*service)
	if !ok {
		return fmt.Errorf("workspace store: hub needs the projection store")
	}
	projections.RegisterHub(svc.store, enrich, broadcast)
	return nil
}

func (s *service) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return s.store.List(ctx, func(context.Context, string) (*domain.Workspace, error) { return nil, nil })
}

func (s *service) Get(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	return s.store.Get(ctx, id)
}

func (s *service) AwaitTombstone(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return s.store.AwaitTombstone(ctx, id)
}

func (s *service) Drop(
	ctx context.Context,
	id string,
) error {
	return s.store.Drop(ctx, id)
}
