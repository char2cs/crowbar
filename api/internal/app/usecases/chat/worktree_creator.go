package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type WorktreeCreator interface {
	CreateChildWorkspace(
		ctx context.Context,
		forkParentID string,
		branch string,
	) (domain.Workspace, error)
	CreateImportedWorkspace(
		ctx context.Context,
		spec ImportSpec,
	) (domain.Workspace, error)
	CreateBranchOutWorkspace(
		ctx context.Context,
		forkParentID string,
		branch string,
	) (domain.Workspace, error)
	CheckBranchOutName(ctx context.Context, forkParentID, branch string) error
	DiscardChildWorkspace(
		ctx context.Context,
		workspaceID string,
	) error
}

var ErrNoForkParent = problem.New(problem.Conflict, "agent: chat has no fork parent to fork from")
