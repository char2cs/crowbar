package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agentRuns"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/conversations"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/kanbanItems"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repositories"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/reviewThreads"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/tasks"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
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
	rg.GET("/fs/file", FsFile)
	git := NewGitHandler(store)
	rg.GET("/git/status", git.Status)
	rg.GET("/git/log", git.Log)
	rg.GET("/git/branches", git.Branches)

	// REST — branch review (mock surfaces the browser serves via MSW)
	rg.GET("/branch-review/:wsId/diff", BranchReviewDiff)
	rg.GET("/branch-review/:wsId/chats", BranchReviewChats)
	rg.GET("/branch-review/:wsId/threads", BranchReviewThreads)
	rg.GET("/branch-review/:wsId/description", BranchReviewDescription)

	// REST — markdown chat
	rg.GET("/markdown-chat/:wsId/:stepId", MarkdownChat)

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
