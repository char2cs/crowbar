// Package handlers holds the gin handler backing the identity endpoint: the
// current human's GitHub/git identity for attributing review comments.
package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
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

// Handlers serves the identity route from the identity resolver and
// workspace reader, mounted on both /v0/chats/:chatId/identity and the older
// /v0/workspaces/:wsId/identity (routes.go).
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

// resolveWorkspace answers which workspace this request acts on, for either
// of the two groups identity is currently mounted on (routes.go).
//
// On /v0/chats/:chatId/identity the chat group's resolveChatWorktree
// middleware has already resolved the chat's worktree and stashed the
// workspace on the context, so the answer is read back from reqscope — never
// resolved a second time per request, and never taken from a URL, because no
// chat-scoped URL carries a workspace id to take it from (spec law 1).
//
// The :wsId branch is the old workspace-scoped mount, unretired until spec §8
// step 6: it still resolves through wsReader, exactly as before this step.
// When that mount goes, so does the branch and the wsReader field with it.
func (h *Handlers) resolveWorkspace(
	ctx *gin.Context,
) (domain.Workspace, error) {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws, nil
	}
	return h.wsReader.Get(ctx.Request.Context(), ctx.Param("wsId"))
}
