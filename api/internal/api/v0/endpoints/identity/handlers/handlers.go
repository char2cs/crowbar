// Package handlers holds the gin handler backing the identity endpoint: the
// current human's GitHub/git identity for attributing review comments.
package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
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

// Handlers serves the identity route from the identity resolver, mounted on
// /v0/chats/:chatId/identity (routes.go).
type Handlers struct {
	identity IdentityResolver
}

// New builds the identity Handlers from the identity resolver.
func New(
	identity IdentityResolver,
) *Handlers {
	return &Handlers{
		identity: identity,
	}
}

// resolveWorkspace answers which workspace this request acts on: the chat
// group's resolveChatWorktree middleware has already resolved the chat's
// worktree and stashed it on the context, so the answer is read back from
// reqscope — never resolved a second time per request, and never taken from a
// URL, because no chat-scoped URL carries a workspace id to take it from
// (spec law 1).
func (h *Handlers) resolveWorkspace(
	ctx *gin.Context,
) (domain.Workspace, error) {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws, nil
	}
	return domain.Workspace{}, apperr.ErrNotFound
}
