// Package terminal registers all terminal REST and WebSocket routes.
package terminal

import (
	"github.com/gin-gonic/gin"

	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

// Register mounts all terminal routes onto rg.
// The WebSocket upgrade for /ws/terminals/:sessionId is served by h.WS.
func Register(
	rg *gin.RouterGroup,
	termEng termhandlers.TerminalEngine,
	profileStore termhandlers.ProfileStore,
	wsReader termhandlers.WorkspaceReader,
) {
	h := termhandlers.New(termEng, profileStore, wsReader)

	rg.POST("/workspaces/:wsId/terminals", h.CreateSession)
	rg.DELETE("/terminals/:sessionId", h.KillSession)

	rg.GET("/settings/terminal/profiles", h.ListProfiles)
	rg.GET("/settings/terminal/profiles/:id", h.GetProfile)
	rg.POST("/settings/terminal/profiles", h.CreateProfile)
	rg.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	rg.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)

	rg.GET("/ws/terminals/:sessionId", h.WS)
}
