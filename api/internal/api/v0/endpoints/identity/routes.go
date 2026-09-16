// Package identity mounts the v0 current-identity REST route. The endpoint
// resolves the current human's GitHub/git identity (login, displayName,
// avatarUrl) for attributing review comments and other authored actions.
package identity

import (
	"github.com/gin-gonic/gin"

	identityhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity/handlers"
)

// Register mounts the identity GET route on the flat chat-scoped group (spec
// §7.1), the only surface identity is addressable through: chats/:chatId/
// identity. identity is spec §4.2's shared bucket — the worktree answers
// once, and every chat holding it gets that answer — resolved from the
// request context by chatScoped's own resolveChatWorktree middleware (see
// handlers.Handlers.resolveWorkspace).
//
// The old /projects/:projectId/repos/:repoId/workspaces/:wsId/identity mount
// is gone (spec §8 step 6): every caller had already moved to the mount kept
// here.
func Register(
	chatScoped *gin.RouterGroup,
	identity identityhandlers.IdentityResolver,
) {
	h := identityhandlers.New(identity)
	chatScoped.GET("/identity", h.Get)
}
