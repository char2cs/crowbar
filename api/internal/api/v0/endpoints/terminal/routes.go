// Package terminal registers all terminal REST and WebSocket routes.
package terminal

import (
	"github.com/gin-gonic/gin"

	termhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
)

// Register mounts the terminal routes across two groups.
//
// The session lifecycle and PTY upgrade routes are CHAT-scoped, so they mount
// on chatScoped (the flat /v0/chats/:chatId group, spec §7.1) and carry
// "/terminals/..."-relative paths. The raw PTY stream is co-located with the
// session routes at .../terminals/:sessionId/ws (W7-2). The terminal profile
// CRUD is a global user setting, so it mounts on settingsRG (the top-level /v0
// group) at /settings/terminal/profiles, outside the entity hierarchy.
//
// Terminal is spec §4.2's "owned by one chat" bucket: the group's
// resolveChatWorktree middleware still runs — a new PTY needs a CWD, and the
// chat's worktree is where it starts — but the session itself is never shared
// with a sibling chat on that same worktree. That is why no WorkspaceReader is
// passed here any more: the middleware already resolved the workspace, and the
// handlers read it off the request context.
func Register(
	chatScoped *gin.RouterGroup,
	settingsRG *gin.RouterGroup,
	termEng termhandlers.TerminalEngine,
	profileStore termhandlers.ProfileStore,
	termBroadcast termhandlers.TerminalBroadcaster,
	wsHandle gin.HandlerFunc,
	dispatch func(rest, ws gin.HandlerFunc) gin.HandlerFunc,
) {
	h := termhandlers.New(termEng, profileStore, termBroadcast)

	// GET .../terminals is dual-served: a plain GET lists the chat's OWN live
	// sessions, while a WebSocket upgrade is routed to the lifecycle broadcaster
	// (D2). POST creates (201 {sessionId}); DELETE kills (202). The raw PTY
	// stream is co-located at .../terminals/:sessionId/ws (W7-2).
	chatScoped.GET("/terminals", dispatch(h.ListSessions, wsHandle))
	chatScoped.POST("/terminals", h.CreateSession)
	chatScoped.DELETE("/terminals/:sessionId", h.KillSession)
	chatScoped.GET("/terminals/:sessionId/ws", h.WS)

	// The host terminal theme is a single global truth (one Crowbar window, one theme), so
	// it mounts on settingsRG beside the profiles rather than under a chat: it is
	// pushed BEFORE any session exists, which is the entire point — see Handlers.SetHostTheme.
	settingsRG.PUT("/settings/terminal/theme", h.SetHostTheme)

	settingsRG.GET("/settings/terminal/profiles", h.ListProfiles)
	settingsRG.GET("/settings/terminal/profiles/:id", h.GetProfile)
	settingsRG.POST("/settings/terminal/profiles", h.CreateProfile)
	settingsRG.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	settingsRG.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
}
