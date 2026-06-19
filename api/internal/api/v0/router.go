package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
	projectsPkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider"
	reposPkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspaces"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// Register mounts the v0 REST and WebSocket routes.
//
// Every entity-scoped route is re-nested under the hierarchical prefix
// /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/... (spec §3). The
// chain is built with gin sub-groups so each endpoint's existing relative paths
// land at the correct depth without per-route prefixing:
//
//	rg            → /v0                                    (health, system, profiles, projects)
//	projectScoped → /v0/projects/:projectId               (repos)
//	repoScoped    → /v0/projects/:projectId/repos/:repoId (workspaces + everything below)
//
// gin requires the wildcard at each tree position to carry a single, consistent
// name: :projectId, :repoId, and :wsId are each defined exactly once by their
// group, so endpoints below them mount "/workspaces/:wsId/..."-relative paths
// without redefining the param.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	// Must be installed before any route registration so every v0 handler
	// chain includes it: requests whose :projectId/:repoId/:wsId matched an
	// empty path segment are rejected with a 400 envelope instead of leaking
	// "" into usecases. The middleware iterates every bound path param, so the
	// deeper nesting introduced here is guarded automatically.
	rg.Use(rejectEmptyPathParams())

	// Top-level, non-entity-scoped routes stay on rg (outside /projects).
	health.Register(rg)
	system.Register(rg)

	projects := rg.Group("/projects")
	projectScoped := projects.Group("/:projectId")
	repos := projectScoped.Group("/repos")
	repoScoped := repos.Group("/:repoId")
	workspacesGrp := repoScoped.Group("/workspaces")
	wsScoped := workspacesGrp.Group("/:wsId")

	projectsPkg.Register(
		rg,
		c.app.Usecases.Project,
		c.app.Usecases.ProjectImport,
		c.app.Usecases.ProjectDelete,
		c.app.Hub.BroadcastProject,
		c.projects.Handle,
		ws.DualServe,
	)
	reposPkg.Register(
		projectScoped,
		c.app.GORM.Repositories,
		c.eng.Provider,
		c.app.Repositories.Workspace,
		c.app.Usecases.ProjectImport,
		c.app.Hub.BroadcastRepo,
		c.repos.Handle,
		ws.DualServe,
	)
	workspaces.Register(
		repoScoped,
		c.app.Usecases.Workspace,
		c.app.Usecases.Worktree,
		c.app.GORM.Repositories,
		c.app.Repositories.Workspace,
		c.workspaces.Handle,
		ws.DualServe,
	)
	// TODO: chat WebSocket surface removed per D11; the chat domain, repo CRUD,
	// and usecase remain dormant. Re-mount chat routes when multi-agent
	// conversations land:
	// /v0/projects/:p/repos/:r/workspaces/:w/chats
	files.Register(
		repoScoped,
		c.app.Usecases.File,
		c.files.Handle,
	)
	git.Register(
		repoScoped,
		c.app.Usecases.Git,
		c.app.Repositories.Workspace,
		c.git.Handle,
		ws.DualServe,
	)
	terminal.Register(
		wsScoped,
		rg,
		c.eng.Terminal,
		c.app.GORM.TerminalProfiles,
		c.app.Repositories.Workspace,
		c.terminals,
		c.terminals.Handle,
		ws.DualServe,
	)
	search.Register(
		repoScoped,
		c.eng.Search,
		c.app.Repositories.Workspace,
	)
	provider.Register(
		repoScoped,
		c.eng.Provider,
		c.app.Repositories.Workspace,
	)
	review.Register(
		repoScoped,
		c.app.Usecases.BranchReview,
	)
	threads.Register(
		repoScoped,
		c.app.Repositories.ReviewThread,
		c.threads,
		c.threads.Handle,
		ws.DualServe,
	)
	editor.Register(
		repoScoped,
		c.eng.LSP,
		c.eng.Git,
		c.app.Repositories.Workspace,
		c.lsp.Handle,
	)
}
