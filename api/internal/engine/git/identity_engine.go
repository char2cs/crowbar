package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// IdentityEngine wraps the package-level CurrentIdentity function so it can be
// passed as a dependency to the identity HTTP handler, which needs an interface
// value rather than a bare function.
type IdentityEngine struct{}

// NewIdentityEngine returns an IdentityEngine ready for use.
func NewIdentityEngine() *IdentityEngine {
	return &IdentityEngine{}
}

// CurrentIdentity resolves the current human's identity for worktreePath via
// the gh CLI (preferred) or git config fallback. It never returns an error.
func (e *IdentityEngine) CurrentIdentity(
	ctx context.Context,
	worktreePath string,
) gitdomain.Identity {
	return CurrentIdentity(ctx, worktreePath)
}
