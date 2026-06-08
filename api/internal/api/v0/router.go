package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// Register mounts the v0 REST and WebSocket routes.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	health.Register(rg)
	projects.Register(
		rg,
		c.app.Usecases.Project,
		c.app.Usecases.ProjectImport,
	)
	// Repo read routes (GET /repos, GET /repos/:id) via Wave 4 endpoint.
	// POST /repos is provided by our handler which does not require repo pre-existence.
	repos.Register(
		rg,
		c.app.GORM.Repositories,
	)
	rg.POST("/repos", c.handleRepoCreate)
	// Workspace, worktree hierarchy, and sync routes via Wave 4 endpoint.
	workspaces.Register(
		rg,
		c.app.Usecases.Workspace,
		c.app.Usecases.Worktree,
		c.app.GORM.Repositories,
		c.workspaces.Handle,
		ws.DualServe,
	)
	registerChatHandlers(rg, c)
	registerFileHandlers(rg, c)
	registerGitReadHandlers(rg, c)
	registerGitWriteHandlers(rg, c)
	// Terminal, search, provider, review, and LSP routes via our handlers.
	registerTerminalHandlers(rg, c)
	registerSearchHandlers(rg, c)
	registerProviderHandlers(rg, c)
	registerReviewHandlers(rg, c)
	// Editor (LSP) routes from Wave 4.
	editor.Register(
		rg,
		c.eng.LSP,
		c.eng.Git,
		c.app.Repositories.Workspace,
	)
	// WebSocket topic routes.
	rg.GET("/ws/workspaces", c.workspaces.Handle)
	rg.GET("/ws/chats", c.chats.Handle)
	rg.GET("/ws/git", c.git.Handle)
	rg.GET("/ws/files", c.files.Handle)
	rg.GET("/ws/lsp", c.lsp.Handle)
	rg.GET("/ws/chats/:chatId/stream", c.chatStream.Handle)
	// AgentRun routes.
	registerAgentRunHandlers(rg, c)
}
