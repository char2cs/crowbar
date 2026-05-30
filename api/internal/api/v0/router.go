package v0

import (
	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/app"
)

func Register(rg *gin.RouterGroup, c *app.Container) {
	store := c.Fixtures
	hubs := c.WSHubs

	// Existing
	rg.GET("/health", NewHealthHandler(c.Health).Check)
	rg.GET("/events", NewEventsHandler(c.Hub).Stream)

	// REST — workspaces
	ws := NewWorkspacesHandler(store)
	rg.GET("/workspaces/:id", ws.Get)
	rg.POST("/workspaces", ws.Create)

	// REST — flows
	rg.GET("/flows", NewFlowsHandler(store).List)

	// REST — conversations
	rg.GET("/conversations/:wsId/:step", NewConversationsHandler(store).Get)

	// REST — projects
	proj := NewProjectsHandler(store)
	rg.GET("/projects", proj.List)
	rg.POST("/projects", proj.Create)

	// REST — fs + git
	rg.GET("/fs/tree", NewFsHandler(store).Tree)
	git := NewGitHandler(store)
	rg.GET("/git/status", git.Status)
	rg.GET("/git/log", git.Log)
	rg.GET("/git/branches", git.Branches)

	// REST — terminal sessions
	rg.POST("/terminal/sessions", NewTerminalHandler().CreateSession)

	// WS channels
	rg.GET("/ws/workspaces", NewWSWorkspacesHandler(hubs.Workspaces, store).Upgrade)
	rg.GET("/ws/git", NewWSGitHandler(hubs.Git, store).Upgrade)
	rg.GET("/ws/files", NewWSFilesHandler(hubs.Files, store).Upgrade)
	rg.GET("/ws/chat/:chatId", NewWSChatHandler(hubs.Chat).Upgrade)
	rg.GET("/ws/terminal/:sessionId", NewWSTerminalHandler(hubs.Terminal).Upgrade)
	rg.GET("/ws/daemon", NewWSDaemonHandler(hubs.Daemon).Upgrade)
}
