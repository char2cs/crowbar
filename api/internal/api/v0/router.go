package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agentrun"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// Register mounts the v0 REST and WebSocket routes.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	// Must be installed before any route registration so every v0 handler
	// chain includes it: requests whose :wsId/:id/etc. matched an empty
	// path segment are rejected with a 400 envelope instead of leaking ""
	// into usecases.
	rg.Use(rejectEmptyPathParams())

	health.Register(rg)
	projects.Register(
		rg,
		c.app.Usecases.Project,
		c.app.Usecases.ProjectImport,
		c.app.Usecases.ProjectDelete,
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
		ws.DualServe,
	)
	chats.Register(
		rg,
		c.app.Usecases.Chat,
		c.app.Repositories.Chat,
		c.chats.Handle,
		c.chatStream.Handle,
	)
	files.Register(
		rg,
		c.app.Usecases.File,
		c.files.Handle,
	)
	git.Register(
		rg,
		c.app.Usecases.Git,
		c.git.Handle,
		ws.DualServe,
	)
	terminal.Register(
		rg,
		c.eng.Terminal,
		c.app.GORM.TerminalProfiles,
		c.app.Repositories.Workspace,
	)
	search.Register(
		rg,
		c.eng.Search,
		c.app.Repositories.Workspace,
	)
	provider.Register(
		rg,
		c.eng.Provider,
		c.app.Repositories.Workspace,
	)
	review.Register(
		rg,
		c.app.Usecases.BranchReview,
	)
	editor.Register(
		rg,
		c.eng.LSP,
		c.eng.Git,
		c.app.Repositories.Workspace,
		c.lsp.Handle,
	)
	agentrun.Register(
		rg,
		c.app.Repositories.AgentRun,
	)
}
