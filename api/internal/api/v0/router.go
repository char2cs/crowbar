package v0

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/browser"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat"
	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/console"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/documents"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
	homePkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/operations"
	pairingPkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/pairing"
	projectsPkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider"
	reposPkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/stream"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspace"
	worktreePkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree"
)

// Register mounts the v0 REST and WebSocket routes on nested gin groups, each defining
// its wildcard once (gin requires one name per tree position):
//
//	rg            → /v0                                    (health, system, profiles, projects)
//	projectScoped → /v0/projects/:projectId               (repos)
//	repoScoped    → /v0/projects/:projectId/repos/:repoId (chats, threads, workspace placement)
//	chatScoped    → /v0/chats/:chatId                     (worktree-scoped surfaces)
//
//nolint:funlen // Flat route-wiring table: one Register call per endpoint group. Splitting it would scatter the mount order across helpers and obscure the nesting the doc comment describes.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	// Installed before any route so every handler chain carries it: gin fixes a
	// route's middleware at registration.
	rg.Use(rejectEmptyPathParams())

	// Open routes go BEFORE requireBearerAuth: gin fixes a route's middleware
	// chain at registration, so these never carry it. Only pairing belongs here
	// (it mints the credential); health is guarded so a revoked token reads 401.
	pairingPkg.RegisterOpen(rg, c.pairing, newSourceLimiter(5, time.Minute, time.Now).handle)

	// Every route from here down needs a bearer token over TLS; plaintext
	// (unix socket, loopback dev TCP) passes through — see requireBearerAuth.
	rg.Use(requireBearerAuth(c.app.GORM.Clients, c.conns))
	health.Register(rg, c.app.Usecases.AgentTurn)
	pairingPkg.RegisterGuarded(rg, c.pairing)
	system.Register(rg)
	operations.Register(rg, c.app.Operations)
	stream.Register(rg, c.app.Stream)
	if c.logs != nil {
		console.Register(rg, c.logs)
	}
	browser.Register(rg, c.app.Browser)

	projects := rg.Group("/projects")
	projectScoped := projects.Group("/:projectId")
	// Installed before the sub-groups are derived so they all inherit it, including
	// the destructive DeleteRepo and icon writes.
	projectScoped.Use(scopeRepoToPath(c.app.GORM.Repositories))
	repos := projectScoped.Group("/repos")
	repoScoped := repos.Group("/:repoId")
	// Installed before the sub-groups are derived so they all inherit it.
	repoScoped.Use(scopeWorkspaceToPath(c.app.Repositories.Workspace))
	// chatScoped is flat because chat ids are globally unique. resolveChatWorktree
	// resolves the chat's worktree once per request for every route mounted on it.
	chats := rg.Group("/chats")
	chatScoped := chats.Group("/:chatId")
	chatScoped.Use(resolveChatWorktree(c.app.Usecases.Worktree))
	chats.GET("/:chatId/blobs/:ref", chathandlers.NewBlobs(c.app.Usecases.AgentChat).Handle)

	projectsPkg.Register(
		rg,
		c.app.Usecases.Project,
		c.app.Usecases.ProjectImport,
		c.app.Usecases.ProjectDelete,
		c.app.Operations,
		c.crowbarHome,
	)
	reposPkg.Register(
		projectScoped,
		c.app.GORM.Repositories,
		c.eng.Provider,
		c.app.Repositories.Workspace,
		c.app.Usecases.ProjectImport,
		c.app.Usecases.Project,
		c.app.RepoPlacements,
		c.eng.Git,
		c.app.Usecases.ProjectDelete,
		c.app.Operations,
		c.crowbarHome,
	)
	homePkg.Register(
		projectScoped,
		c.app.Repositories.Workspace,
		c.app.Usecases.File,
		c.eng.Terminal,
		// The same Working seam the chat list's worktree fields stamp from, so GET /home
		// agrees with the frames broadcast for the home workspace.
		c.app.Repositories,
		// Reused from the workspace-scoped surface: the review-thread store and
		// its publisher. home.Register injects the resolved home :wsId so these
		// scope correctly.
		c.app.Repositories.ReviewThread,
		threadStream{c.app.Stream},
		// The chat and chat-folder routes are re-mounted under the home group so
		// a project home has chats and folders too; home.Register injects the
		// resolved home :wsId.
		c.app.Usecases.AgentChat,
		c.app.Usecases.AgentTurn,
		c.app.Usecases.AgentRunner,
		c.app.Usecases.AgentAnswer,
		c.app.Usecases.AgentProvider,
		c.app.Usecases.AgentChatFolder,
		chatWorktrees{app: c.app},
		c.app.Repositories.Placement,
	)
	// The worktree surface a chat addresses: lifecycle verbs, branch rename, and
	// batch branch import.
	worktreePkg.Register(
		repoScoped,
		c.app.Usecases.Workspace,
		c.app.Usecases.Workspace,
		c.app.GORM.Repositories,
		c.app.Operations,
		c.app.Usecases.AgentChat,
		c.eng.Git,
		// The same resolver chatScoped's middleware uses, so verbs and reads agree on
		// which worktree a chat holds.
		c.app.Usecases.Worktree,
	)
	// Files is shared: every chat holding a worktree reads the same tree. The home
	// group keeps its own /home/files for the project row no chat resolves to.
	files.Register(
		chatScoped,
		c.app.Usecases.File,
	)
	// A document's content is read and written by ref on the flat /v0 group:
	// a host file outside every worktree has no chat or workspace to scope by.
	documents.Register(rg, c.app.Usecases.File)
	// Git is shared: every chat holding a worktree sees the same writes.
	git.Register(
		chatScoped,
		c.app.Usecases.Git,
		c.app.Operations,
	)
	// Terminal reads the PTY's CWD from the workspace chatScoped resolved.
	terminal.Register(
		chatScoped,
		rg,
		c.eng.Terminal,
		c.app.GORM.TerminalProfiles,
	)
	// Chat routes are repo-scoped: a chat's workspace is optional and mutable,
	// so no chat route names one. rg carries the global provider-preferences
	// route (/settings/chat/providers).
	chat.Register(
		repoScoped,
		chatScoped,
		rg,
		c.app.Usecases.AgentChat,
		c.app.Usecases.AgentTurn,
		c.app.Usecases.AgentRunner,
		c.app.Usecases.AgentAnswer,
		c.app.Usecases.AgentProvider,
		c.app.Usecases.AgentChatFolder,
		// Read only by POST /chats importing a branch, which needs the repo's path
		// and remote.
		c.app.GORM.Repositories,
		// The git fields a worktree-owning chat carries on its DTO. The home group
		// mounts the same adapter so its owner row says it owns the home worktree.
		chatWorktrees{app: c.app},
		// A worktree-owning chat's sidebar FolderID/Order: the store PlaceWorkspace writes.
		c.app.Repositories.Placement,
	)
	// Search, review and identity are shared: every chat holding a worktree sees the
	// same reads.
	search.Register(
		chatScoped,
		c.eng.Search,
	)
	// Provider state is per chat and never shared with a sibling;
	// /protected-branches is repo-level, so it mounts on repoScoped.
	provider.Register(
		repoScoped,
		chatScoped,
		c.eng.Provider,
		c.app.Repositories.Workspace,
	)
	review.Register(
		chatScoped,
		c.app.Usecases.BranchReview,
	)
	threads.Register(
		repoScoped,
		c.app.Repositories.ReviewThread,
		threadStream{c.app.Stream},
	)
	// A locked branch's sidebar position is a fact about the workspace, not a chat,
	// so it is addressed by workspace id; it shares PlaceChat's tree usecase.
	workspace.Register(
		repoScoped,
		c.app.Usecases.AgentChatFolder,
	)
	// Editor/LSP uses the resolved worktree as its CWD, but the LSP session is never
	// shared with a sibling chat holding the same worktree.
	editor.Register(
		chatScoped,
		c.eng.LSP,
		c.eng.Git,
	)
	identity.Register(
		chatScoped,
		c.eng.Identity,
	)
}

// crowbarHome is the resolver the icon handlers store and serve through: the
// home the app layer was opened against.
func (c *Container) crowbarHome() (string, error) {
	return c.app.CrowbarHome(), nil
}
