// Package terminal registers all terminal REST and WebSocket routes.
package terminal

import (
	"github.com/gin-gonic/gin"

	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

// Register mounts the terminal routes across two groups.
//
// The session lifecycle and PTY upgrade routes are workspace-scoped, so they
// mount on repoScoped (the /v0/projects/:projectId/repos/:repoId group) and
// carry the "/workspaces/:wsId/..."-relative paths. The terminal profile CRUD
// is a global user setting, so it mounts on settingsRG (the top-level /v0
// group) at /settings/terminal/profiles, outside the entity hierarchy.
func Register(
	repoScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	termEng termhandlers.TerminalEngine,
	profileStore termhandlers.ProfileStore,
	wsReader termhandlers.WorkspaceReader,
) {
	h := termhandlers.New(termEng, profileStore, wsReader)

	repoScoped.POST("/workspaces/:wsId/terminals", h.CreateSession)
	repoScoped.DELETE("/terminals/:sessionId", h.KillSession)
	repoScoped.GET("/ws/terminals/:sessionId", h.WS)

	settingsRG.GET("/settings/terminal/profiles", h.ListProfiles)
	settingsRG.GET("/settings/terminal/profiles/:id", h.GetProfile)
	settingsRG.POST("/settings/terminal/profiles", h.CreateProfile)
	settingsRG.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	settingsRG.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
}
