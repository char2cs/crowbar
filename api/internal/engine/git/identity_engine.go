package git

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type IdentityEngine struct{}

func NewIdentityEngine() *IdentityEngine {
	return &IdentityEngine{}
}

func (e *IdentityEngine) CurrentIdentity(
	ctx context.Context,
	worktreePath string,
) gitdomain.Identity {
	return CurrentIdentity(ctx, worktreePath)
}
