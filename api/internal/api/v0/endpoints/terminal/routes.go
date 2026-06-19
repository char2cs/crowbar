// Package terminal registers all terminal REST and WebSocket routes.
package terminal

import (
	"github.com/gin-gonic/gin"

	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

// Register mounts the terminal routes across two groups.
//
// The session lifecycle and PTY upgrade routes are workspace-scoped, so they
// mount on wsScoped (the /v0/projects/:projectId/repos/:repoId/workspaces/:wsId
// group) and carry "/terminals/..."-relative paths. The raw PTY stream is
// co-located with the session routes at .../terminals/:sessionId/ws (W7-2). The
// terminal profile CRUD is a global user setting, so it mounts on settingsRG
// (the top-level /v0 group) at /settings/terminal/profiles, outside the
// entity hierarchy.
func Register(
	wsScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	termEng termhandlers.TerminalEngine,
	profileStore termhandlers.ProfileStore,
	wsReader termhandlers.WorkspaceReader,
) {
	h := termhandlers.New(termEng, profileStore, wsReader)

	wsScoped.POST("/terminals", h.CreateSession)
	wsScoped.DELETE("/terminals/:sessionId", h.KillSession)
	wsScoped.GET("/terminals/:sessionId/ws", h.WS)

	settingsRG.GET("/settings/terminal/profiles", h.ListProfiles)
	settingsRG.GET("/settings/terminal/profiles/:id", h.GetProfile)
	settingsRG.POST("/settings/terminal/profiles", h.CreateProfile)
	settingsRG.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	settingsRG.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
}
