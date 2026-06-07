// Package terminal mounts the v0 terminal REST routes plus the bidirectional PTY
// WebSocket (00 §5.6). The REST surface covers terminal-profile CRUD under
// /settings/terminal/profiles and the PTY session lifecycle (create under
// /workspaces/:wsId/terminals, kill under /terminals/:sessionId), all wrapped in
// the {success,error,data} envelope. The WebSocket route
// /ws/terminals/:sessionId is injected as a ready gin.HandlerFunc because it
// speaks a raw PTY frame protocol rather than the REST envelope; build it with
// handlers.NewWS.
package terminal

import (
	"github.com/gin-gonic/gin"

	terminalhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

// Register mounts the terminal REST routes and the PTY WebSocket on the supplied
// router group, backed by the terminal engine, the profile store, and the
// workspace reader. termWS is the pre-built WebSocket handler (see
// handlers.NewWS) mounted at /ws/terminals/:sessionId.
func Register(
	rg *gin.RouterGroup,
	termEng terminalhandlers.TerminalEngine,
	profileStore terminalhandlers.ProfileStore,
	wsReader terminalhandlers.WorkspaceReader,
	termWS gin.HandlerFunc,
) {
	h := terminalhandlers.New(termEng, profileStore, wsReader)

	rg.POST("/workspaces/:wsId/terminals", h.CreateSession)
	rg.DELETE("/terminals/:sessionId", h.KillSession)

	rg.GET("/ws/terminals/:sessionId", termWS)

	rg.GET("/settings/terminal/profiles", h.ListProfiles)
	rg.GET("/settings/terminal/profiles/:id", h.GetProfile)
	rg.POST("/settings/terminal/profiles", h.CreateProfile)
	rg.PATCH("/settings/terminal/profiles/:id", h.UpdateProfile)
	rg.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
}
