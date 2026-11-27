package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type IdentityResolver interface {
	CurrentIdentity(
		ctx context.Context,
		worktreePath string,
	) gitdomain.Identity
}

type Handlers struct {
	identity IdentityResolver
}

func New(
	identity IdentityResolver,
) *Handlers {
	return &Handlers{
		identity: identity,
	}
}

func (h *Handlers) resolveWorkspace(
	ctx *gin.Context,
) (domain.Workspace, error) {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws, nil
	}
	return domain.Workspace{}, problem.ErrNotFound
}
