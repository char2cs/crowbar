// Package provider mounts the v0 provider REST routes: workspace provider state
// poll and protected-branch lookup.
package provider

import (
	"github.com/gin-gonic/gin"

	provhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
)

// Register mounts GET /v0/chats/:chatId/provider (the per-worktree provider
// poll) and /protected-branches on repoScoped, since protection is repo-level.
func Register(
	repoScoped *gin.RouterGroup,
	chatScoped *gin.RouterGroup,
	provEng provhandlers.ProviderEngine,
	wsReader provhandlers.WorkspaceReader,
) {
	h := provhandlers.New(provEng, wsReader)
	chatScoped.GET("/provider", h.State)
	repoScoped.GET("/protected-branches", h.ProtectedBranches)
}
