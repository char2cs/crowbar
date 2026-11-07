package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// WorktreeCreator is the narrow write port this feature needs from the worktree
// hierarchy usecase, declared by the consumer so the worktree usecase's wide surface
// never leaks in.
type WorktreeCreator interface {
	// CreateChildWorkspace forks a fresh workspace off forkParentID; a blank branch
	// gets a server-generated name.
	CreateChildWorkspace(
		ctx context.Context,
		forkParentID string,
		branch string,
	) (domain.Workspace, error)
	// CreateImportedWorkspace is CreateChildWorkspace's import counterpart: the
	// workspace for a branch that already exists, named outright by spec.
	CreateImportedWorkspace(
		ctx context.Context,
		spec ImportSpec,
	) (domain.Workspace, error)
	// CreateBranchOutWorkspace forks a fresh workspace off forkParentID at the commit
	// its checkout holds, never origin's tip; a blank branch gets a generated name.
	CreateBranchOutWorkspace(
		ctx context.Context,
		forkParentID string,
		branch string,
	) (domain.Workspace, error)
	// DiscardChildWorkspace removes a just-minted workspace whose create then failed:
	// a workspace is reachable only through its owning row, so an abandoned one
	// would be a worktree on disk nothing can name again.
	DiscardChildWorkspace(
		ctx context.Context,
		workspaceID string,
	) error
}

// ErrNoForkParent is returned when chatID has no ancestor carrying a
// workspace to fork from: a bubble at the panel root, or one whose every
// ancestor is workspace-less too.
var ErrNoForkParent = problem.New(problem.Conflict, "agent: chat has no fork parent to fork from")
