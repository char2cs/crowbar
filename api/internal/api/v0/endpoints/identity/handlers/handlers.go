// Package handlers holds the gin handler backing the identity endpoint: the
// current human's GitHub/git identity for attributing review comments.
package handlers

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// IdentityResolver resolves the current human's identity from a worktree path.
// It is satisfied by a closure wrapping enginegit.CurrentIdentity.
type IdentityResolver interface {
	// CurrentIdentity resolves the human's identity for worktreePath.
	// It must never return a hard error — a best-effort (possibly empty)
	// Identity is always returned.
	CurrentIdentity(
		ctx context.Context,
		worktreePath string,
	) gitdomain.Identity
}

// WorkspaceReader resolves a workspace id to its aggregate, supplying the
// worktree path the identity resolver operates against.
type WorkspaceReader interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Handlers serves the /v0 identity route from the identity resolver and
// workspace reader.
type Handlers struct {
	identity IdentityResolver
	wsReader WorkspaceReader
}

// New builds the identity Handlers from the identity resolver and workspace
// reader.
func New(
	identity IdentityResolver,
	wsReader WorkspaceReader,
) *Handlers {
	return &Handlers{
		identity: identity,
		wsReader: wsReader,
	}
}
