package workspace

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
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
}

type workspace struct {
	ax asynx.Asynx[domain.Workspace]
}

// New builds a Workspace repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.Workspace],
) Workspace {
	return &workspace{ax: ax}
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
