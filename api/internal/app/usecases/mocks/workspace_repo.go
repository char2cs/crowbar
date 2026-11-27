package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type WorkspaceRepo struct {
	Created   []domain.Workspace
	CreateErr error
	Deleted   []string
	DeleteErr error
	CreateFn  func(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error)
}

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
