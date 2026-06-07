package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces"
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
	repos.Register(
		rg,
		c.app.GORM.Repositories,
	)
	workspaces.Register(
		rg,
		c.app.Usecases.Workspace,
		c.app.Usecases.Worktree,
		c.app.GORM.Repositories,
		c.workspaces.Handle,
		dualServe,
	)
	chats.Register(
		rg,
		c.app.Usecases.Chat,
	)
	files.Register(
		rg,
		c.app.Usecases.File,
	)
	editor.Register(
		rg,
		c.eng.LSP,
		c.eng.Git,
		c.app.Repositories.Workspace,
	)
	git.Register(
		rg,
		c.app.Usecases.Git,
		c.git.Handle,
		dualServe,
	)
	rg.GET("/ws/workspaces", c.workspaces.Handle)
	rg.GET("/ws/chats", c.chats.Handle)
	rg.GET("/ws/git", c.git.Handle)
	rg.GET("/ws/files", c.files.Handle)
	rg.GET("/ws/lsp", c.lsp.Handle)
	registerTerminalHandlers(rg, c)
	registerSearchHandlers(rg, c)
	registerProviderHandlers(rg, c)
	review.Register(
		rg,
		c.app.Usecases.BranchReview,
	)
}
