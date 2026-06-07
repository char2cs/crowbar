// Package handlers holds the gin handlers backing the workspaces endpoint: the
// flat list and detail reads, worktree-backed create and cascade delete, and
// the hierarchy operations (local merge-into-parent and reparent).
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Reader is the workspace read surface the handlers need: list every workspace
// row from the read model and fetch one by id.
type Reader interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Hierarchy is the worktree-orchestration surface the handlers need: create a
// worktree-backed child, cascade-delete a subtree, run a local child→parent
// merge, and reparent a leaf child onto a new parent (07).
type Hierarchy interface {
	CreateChild(
		ctx context.Context,
		in worktree.CreateChildInput,
	) (domain.Workspace, error)
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (worktree.MergeResult, error)
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	DeleteCascade(
		ctx context.Context,
		rootID string,
	) error
}

// Repos resolves a repository by id so the create handler can derive the repo's
// on-disk path and owning project from the request's repoId.
type Repos interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Repository, error)
}

// Handlers serves the /v0/workspaces routes from the workspace read usecase, the
// worktree hierarchy usecase, and the repository store.
type Handlers struct {
	reader    Reader
	hierarchy Hierarchy
	repos     Repos
}

// New builds the workspaces Handlers from the workspace read usecase, the
// worktree hierarchy usecase, and the repository store.
func New(
	reader Reader,
	hierarchy Hierarchy,
	repos Repos,
) *Handlers {
	return &Handlers{
		reader:    reader,
		hierarchy: hierarchy,
		repos:     repos,
	}
}
