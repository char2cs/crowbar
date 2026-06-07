// Package handlers holds the gin handlers backing the git read endpoint: the
// working-tree status (dual-served with the live git stream), the paginated
// log, the working-tree or commit diff, the branch list, and the stash list
// (02 §2.6).
//
// Git write and conflict operations are served by separate endpoints; this
// package is read-only.
package handlers

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Git is the git read surface the handlers need: the working-tree status, the
// working-tree (or staged) diff, a single commit's diff, the paginated log, the
// branch list, and the stash list. It mirrors the read half of the git usecase
// so the live usecase satisfies it directly.
type Git interface {
	Status(
		ctx context.Context,
		wsID string,
	) (gitdomain.GitStatus, error)
	Diff(
		ctx context.Context,
		wsID string,
		staged bool,
	) ([]gitdomain.FileDiff, error)
	CommitDiff(
		ctx context.Context,
		wsID string,
		sha string,
	) (gitdomain.MultiFileDiff, error)
	Log(
		ctx context.Context,
		wsID string,
		limit int,
		skip int,
	) ([]gitdomain.Commit, error)
	Branches(
		ctx context.Context,
		wsID string,
	) ([]gitdomain.Branch, error)
	Stashes(
		ctx context.Context,
		wsID string,
	) ([]gitdomain.Stash, error)
}

// Handlers serves the /v0/workspaces/:wsId/git read routes from the git usecase.
type Handlers struct {
	git Git
}

// New builds the git Handlers from the git read usecase.
func New(
	git Git,
) *Handlers {
	return &Handlers{
		git: git,
	}
}
