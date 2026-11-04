package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// WorkspaceRepo is a fake of the subset of workspace.Workspace used on import.
type WorkspaceRepo struct {
	Created   []domain.Workspace
	CreateErr error
	// Deleted records the rows a rollback tombstoned, in order, so a test can
	// tell a workspace that was taken back out from one silently left behind.
	Deleted   []string
	DeleteErr error
	// CreateFn, when non-nil, is called instead of the default stub logic. The
	// caller is responsible for appending to Created if it wants tracking.
	CreateFn func(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error)
}

// NewWorkspaceRepo returns an empty WorkspaceRepo.
func NewWorkspaceRepo() *WorkspaceRepo {
	return &WorkspaceRepo{}
}

func (r *WorkspaceRepo) Create(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	if r.CreateFn != nil {
		return r.CreateFn(ctx, in, now)
	}
	if r.CreateErr != nil {
		return domain.Workspace{}, r.CreateErr
	}
	status := domain.WorkspaceStatusNew
	if in.Protected {
		status = domain.WorkspaceStatusLocked
	}
	ws := domain.Workspace{
		ID:            in.ID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  in.WorktreePath,
		ForkPointSha:  in.ForkPointSha,
		ParentID:      in.ParentID,
		Status:        status,
		MergeStrategy: in.MergeStrategy,
		IsDefault:     in.IsDefault,
		Kind:          in.Kind,
		HeldByPath:    in.HeldByPath,
		Provisioning:  in.Provisioning,
		CreatedAt:     now,
	}
	r.Created = append(r.Created, ws)
	return ws, nil
}

// Delete tombstones a created row, recording the id so a test can prove a
// workspace that could not be attached to its owning chat was taken back out
// rather than left orphaned.
func (r *WorkspaceRepo) Delete(
	_ context.Context,
	id string,
) error {
	if r.DeleteErr != nil {
		return r.DeleteErr
	}
	r.Deleted = append(r.Deleted, id)
	return nil
}
