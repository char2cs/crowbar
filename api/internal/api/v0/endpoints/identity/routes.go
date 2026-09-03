// Package identity mounts the v0 current-identity REST route. The endpoint
// resolves the current human's GitHub/git identity (login, displayName,
// avatarUrl) for attributing review comments and other authored actions.
package identity

import (
	"github.com/gin-gonic/gin"

	identityhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity/handlers"
)

// Register mounts the identity GET route on BOTH scoping groups identity is
// currently addressable through.
//
// identity is spec §4.2's shared bucket: the worktree answers once, and every
// chat holding it gets that answer. chatScoped is where that lives from now
// on — /v0/chats/:chatId/identity, the flat prefix §7.1 closes on — and the
// frontend talks to it exclusively.
//
// wsScoped is the OLD /projects/:projectId/repos/:repoId/workspaces/:wsId/
// identity surface, mounted unchanged. It is not a fallback and nothing
// chooses between the two: it is simply a route that has not been retired
// yet, and retiring it is spec §8 step 6's job, once every group has moved
// and the workspaces/home groups are deleted wholesale. Deleting THIS call is
// the whole of identity's share of that step.
//
// A single Handlers value serves both mounts, so the two can never drift into
// different behaviour. The handler itself resolves the workspace from
// whichever mount the request arrived on — see handlers.Handlers.resolveWorkspace.
func Register(
	wsScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	identity identityhandlers.IdentityResolver,
	wsReader identityhandlers.WorkspaceReader,
) {
	h := identityhandlers.New(identity, wsReader)
	chatScoped.GET("/identity", h.Get)
	wsScoped.GET("/workspaces/:wsId/identity", h.Get)
}
