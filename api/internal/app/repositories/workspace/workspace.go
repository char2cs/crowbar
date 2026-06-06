package workspace

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// CreateInput carries the fields needed to create a workspace.
type CreateInput struct {
	ID            string
	RepoID        string
	ProjectID     string
	Branch        string
	WorktreePath  string
	ForkPointSha  string
	ParentID      string
	Locked        bool
	MergeStrategy gitdomain.MergeStrategy
}

// SyncInput carries a recomputed working-tree summary.
type SyncInput struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

// ProviderInput carries a provider poll result (08 §5).
type ProviderInput struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

// Workspace is the workspace aggregate repository.
type Workspace interface {
	Create(
		ctx context.Context,
		in CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SyncProviderState(
		ctx context.Context,
		in ProviderInput,
		now time.Time,
	) (domain.Workspace, error)
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	TouchActivity(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	Reparent(
		ctx context.Context,
		id string,
		parentID string,
		forkPointSha string,
		now time.Time,
	) (domain.Workspace, error)
	UpdateForkPoint(
		ctx context.Context,
		id string,
		forkPointSha string,
	) (domain.Workspace, error)
	SetPendingMerge(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
		targetParentID string,
	) (domain.Workspace, error)
	ClearPendingMerge(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

type workspace struct {
	ax    asynx.Asynx[domain.Workspace]
	store store.Store
}

// New builds a Workspace repository over the asynx instance and a GORM DB. The
// broadcast func is the hub fan-out for projected rows (03 §2).
func New(
	ax asynx.Asynx[domain.Workspace],
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (Workspace, error) {
	st, err := store.New(db, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("workspace: store: %w", err)
	}
	return &workspace{ax: ax, store: st}, nil
}

func (w *workspace) Create(
	ctx context.Context,
	in CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.CreateWorkspace{
		ID:            in.ID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  in.WorktreePath,
		ForkPointSha:  in.ForkPointSha,
		ParentID:      in.ParentID,
		Locked:        in.Locked,
		MergeStrategy: in.MergeStrategy,
		Now:           now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SyncWorkingTreeState(
	ctx context.Context,
	in SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SyncWorkingTreeState{
		ID:           in.ID,
		Added:        in.Added,
		Deleted:      in.Deleted,
		HasConflicts: in.HasConflicts,
		HasCommits:   in.HasCommits,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	got, err := w.ax.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	return got, nil
}

func (w *workspace) SyncProviderState(
	ctx context.Context,
	in ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SyncProviderState{
		ID:             in.ID,
		Protected:      in.Protected,
		HasPR:          in.HasPR,
		PRStatus:       in.PRStatus,
		PRUrl:          in.PRUrl,
		PRTitle:        in.PRTitle,
		PRTargetBranch: in.PRTargetBranch,
		Now:            now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync provider: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SetMergeStrategy{ID: id, Strategy: strategy})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set merge strategy: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) TouchActivity(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.TouchActivity{ID: id, Now: now})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: touch activity: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Reparent(
	ctx context.Context,
	id string,
	parentID string,
	forkPointSha string,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.Reparent{
		ID:           id,
		ParentID:     parentID,
		ForkPointSha: forkPointSha,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: reparent: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.UpdateForkPoint{ID: id, ForkPointSha: forkPointSha})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: update fork point: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetPendingMerge(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
	targetParentID string,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SetPendingMerge{
		ID:             id,
		Strategy:       strategy,
		TargetParentID: targetParentID,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set pending merge: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ClearPendingMerge(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.ClearPendingMerge{ID: id})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: clear pending merge: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Delete(
	ctx context.Context,
	id string,
) error {
	if err := w.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("workspace: delete: %w", err)
	}
	return nil
}

// List returns all workspace rows from the read-model projection.
func (w *workspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := w.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return rows, nil
}
