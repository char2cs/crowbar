package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
	foldersPkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/folders"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
	homePkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity"
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
//
//nolint:funlen // Flat route-wiring table: one Register call per endpoint group. Splitting it would scatter the mount order across helpers and obscure the nesting the doc comment describes.
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
	// Enforce :repoId ⊂ :projectId for every repo- and workspace-scoped route.
	// Installed on projectScoped BEFORE its sub-groups so they all inherit it; a
	// request whose :repoId belongs to a different project is rejected 404 before
	// any handler (incl. the destructive DeleteRepo / icon writes). Routes with no
	// :repoId pass through.
	projectScoped.Use(scopeRepoToPath(c.app.GORM.Repositories))
	repos := projectScoped.Group("/repos")
	repoScoped := repos.Group("/:repoId")
	// Enforce :wsId ⊂ :projectId/:repoId for every entity-scoped route. Installed
	// on repoScoped BEFORE its sub-groups are derived so they all inherit it; a
	// request whose :wsId belongs to a different project/repo is rejected 404
	// before any handler runs. Routes with no :wsId pass through untouched.
	repoScoped.Use(scopeWorkspaceToPath(c.app.Repositories.Workspace))
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
		c.app.Usecases.Project,
		c.eng.Git,
		c.app.Usecases.Worktree,
		c.app.Repositories.Workspace,
		c.app.Hub.BroadcastRepo,
		c.repos.Handle,
		ws.DualServe,
	)
	homePkg.Register(
		projectScoped,
		c.app.Repositories.Workspace,
		c.app.GORM.Projects,
		c.app.Usecases.File,
		c.eng.Terminal,
		// The working-overlay read seam, the SAME one workspaces.Register stamps its
		// list/detail reads from (the repositories Container's WorkingFor, which ORs
		// the inflight-mutation and agent-turn overlays). GET /home is the home
		// workspace's only REST read, so it stamps Working from here to agree with the
		// frames the container broadcasts for that same workspace.
		c.app.Repositories,
		// Reused from the workspace-scoped surface: the file-change WS handler and
		// the review-thread store/broadcaster/WS, dual-served via the same wrapper.
		// home.Register injects the resolved home :wsId so these scope correctly.
		c.files.Handle,
		c.app.Repositories.ReviewThread,
		c.threads,
		c.threads.Handle,
		// The agent chat surface (REST + lifecycle WS) is re-mounted under the
		// home group so project-home workspaces get agentic chats too (the same
		// usecase + WS broadcaster agent.Register uses on the workspace-scoped
		// group); home.Register injects the resolved home :wsId so both scope
		// correctly. The chat-FOLDER surface is re-mounted with it, for the same
		// reason and then some: the home accumulates the most chats of any
		// workspace, so a home without folders is the one place the panel most
		// needs them and would not have them.
		c.app.Usecases.Agent,
		c.app.Usecases.AgentChatFolder,
		c.app.Hub.BroadcastAgentChatFolder,
		c.agentChats.Handle,
		ws.DualServe,
	)
	workspaces.Register(
		repoScoped,
		c.app.Usecases.Workspace,
		c.app.Usecases.Worktree,
		c.app.GORM.Repositories,
		c.app.Repositories.Workspace,
		c.app.Repositories,
		c.eng.Git,
		c.app.Usecases.Folder,
		c.app.Hub.BroadcastFolder,
		c.workspaces.Handle,
		ws.DualServe,
	)
	foldersPkg.Register(
		repoScoped,
		c.app.Usecases.Folder,
		c.app.Hub.BroadcastFolder,
		c.folders.Handle,
		ws.DualServe,
	)
	files.Register(
		repoScoped,
		c.app.Usecases.File,
		c.files.Handle,
	)
	git.Register(
		repoScoped,
		c.app.Usecases.Git,
		c.app.Repositories.Workspace,
		c.app.Repositories,
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
	// The agentic-chat REST + WS surface is workspace-scoped (Task 3): every
	// AgentChat is anchored to a workspace, so its routes mount on wsScoped
	// (.../workspaces/:wsId) exactly like terminal.Register above, giving it
	// scopeWorkspaceToPath's wsId-ownership enforcement for free. The WS route
	// (.../agent/ws/chats) lands in the SAME group as the REST routes so its
	// :wsId path param is available to agentChatDef's Filter (container.go). rg
	// carries the GLOBAL provider-preferences write route (/settings/agent/providers),
	// mounted once outside the entity hierarchy like /settings/terminal/profiles.
	agent.Register(
		wsScoped,
		rg,
		c.app.Usecases.Agent,
		c.app.Usecases.AgentChatFolder,
		c.app.Hub.BroadcastAgentChatFolder,
		c.agentChats.Handle,
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
	identity.Register(
		repoScoped,
		c.eng.Identity,
		c.app.Repositories.Workspace,
	)
}
