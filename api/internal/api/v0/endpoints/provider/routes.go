// Package provider mounts the v0 provider REST routes: workspace provider state
// poll and protected-branch lookup.
package provider

import (
	"github.com/gin-gonic/gin"

	provhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
)

// Register mounts the provider routes on the supplied router groups.
//
// State (the workspace provider-poll route) is spec §4.2's OWNED bucket, same
// as editor and terminal: the resolver still runs, for a CWD, but the poll
// itself answers per request rather than keying any session, so it needs no
// analogue of editor's lspOwnerID. chatScoped is where it lives from now on —
// GET /v0/chats/:chatId/provider — and the frontend talks to it exclusively.
// wsScoped is the OLD .../workspaces/:wsId/provider surface, mounted
// unchanged; it is not a fallback, and retiring it is spec §8 step 6's job.
//
// /protected-branches does NOT move (spec §4.2 is explicit: it is repo-level,
// not worktree-owned) and stays exactly where it always was, on wsScoped's
// repo-level ancestor.
func Register(
	wsScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	provEng provhandlers.ProviderEngine,
	wsReader provhandlers.WorkspaceReader,
) {
	h := provhandlers.New(provEng, wsReader)
	chatScoped.GET("/provider", h.State)
	wsScoped.GET("/workspaces/:wsId/provider", h.State)
	wsScoped.GET("/protected-branches", h.ProtectedBranches)
}
