package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// WorkspaceSyncer is a fake of the workspace syncer surface used by the file and
// git usecases.
type WorkspaceSyncer struct {
	Synced     bool
	SyncedID   string
	SyncedIDs  []string
	Resolved   bool
	ResolvedID string

	GetFn     func(ctx context.Context, id string) (domain.Workspace, error)
	ListFn    func(ctx context.Context) ([]domain.Workspace, error)
	SyncFn    func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
	ResolveFn func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
}

// NewWorkspaceSyncer returns an empty WorkspaceSyncer.
func NewWorkspaceSyncer() *WorkspaceSyncer {
	return &WorkspaceSyncer{}
}

func (s *WorkspaceSyncer) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return s.GetFn(ctx, id)
}

// List returns the fake's workspace rows, defaulting to none when no ListFn is
// set (the cascade then finds no children — the pull/fetch resyncs only itself).
func (s *WorkspaceSyncer) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	if s.ListFn != nil {
		return s.ListFn(ctx)
	}
	return nil, nil
}

func (s *WorkspaceSyncer) SyncWorkingTreeState(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	s.Synced = true
	s.SyncedID = id
	s.SyncedIDs = append(s.SyncedIDs, id)
	if s.SyncFn != nil {
		return s.SyncFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}

func (s *WorkspaceSyncer) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	s.Resolved = true
	s.ResolvedID = id
	if s.ResolveFn != nil {
		return s.ResolveFn(ctx, id, now)
	}
	return domain.Workspace{ID: id}, nil
}
