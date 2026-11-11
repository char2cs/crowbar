// Package terminal registers all terminal REST and WebSocket routes.
package terminal

import (
	"github.com/gin-gonic/gin"

	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

// Register mounts the session lifecycle and PTY stream (.../terminals/:sessionId/ws) on chatScoped,
// and the global profile CRUD and host theme on settingsRG. A session is owned by one chat and
// starts in that chat's worktree; sibling chats on the same worktree never share it.
func Register(
	chatScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	termEng termhandlers.TerminalEngine,
	profileStore termhandlers.ProfileStore,
) {
	h := termhandlers.New(termEng, profileStore)

	// GET .../terminals lists the chat's OWN live sessions. POST creates (201
	// {sessionId}); DELETE kills (200). The raw PTY stream is co-located at
	// .../terminals/:sessionId/ws.
	chatScoped.GET("/terminals", h.ListSessions)
	chatScoped.POST("/terminals", h.CreateSession)
	chatScoped.DELETE("/terminals/:sessionId", h.KillSession)
	chatScoped.GET("/terminals/:sessionId/ws", h.WS)

	// The host terminal theme is a single global truth (one Crowbar window, one theme), so
	// it mounts on settingsRG beside the profiles rather than under a chat: it is
	// pushed BEFORE any session exists, which is the entire point — see Handlers.SetHostTheme.
	settingsRG.GET("/settings/terminal/theme", h.HostTheme)
	settingsRG.PUT("/settings/terminal/theme", h.SetHostTheme)

	settingsRG.GET("/settings/terminal/profiles", h.ListProfiles)
	settingsRG.GET("/settings/terminal/profiles/:id", h.GetProfile)
	settingsRG.POST("/settings/terminal/profiles", h.CreateProfile)
	settingsRG.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	settingsRG.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
}
