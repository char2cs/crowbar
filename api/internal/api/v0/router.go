package v0

import (
	"time"

	"github.com/gin-gonic/gin"

	accessPkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/access"
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

//nolint:funlen // Flat route-wiring table: one Register call per endpoint group. Splitting it would scatter the mount order across helpers and obscure the nesting the doc comment describes.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	rg.Use(rejectEmptyPathParams())

	pairingPkg.RegisterOpen(rg, c.pairing, newSourceLimiter(5, time.Minute, time.Now).handle)

	rg.Use(requireBearerAuth(c.app.GORM.Clients, c.conns))
	health.Register(rg, c.app.Usecases.AgentTurn)
	pairingPkg.RegisterGuarded(rg, c.pairing)
	accessPkg.Register(rg, c.access)
	system.Register(rg)
	operations.Register(rg, c.app.Operations)
	stream.Register(rg, c.app.Stream)
	if c.logs != nil {
		console.Register(rg, c.logs)
	}
	browser.Register(rg, c.app.Browser)

	projects := rg.Group("/projects")
	projectScoped := projects.Group("/:projectId")
	projectScoped.Use(scopeRepoToPath(c.app.GORM.Repositories))
	repos := projectScoped.Group("/repos")
	repoScoped := repos.Group("/:repoId")
	repoScoped.Use(scopeWorkspaceToPath(c.app.Repositories.Workspace))
	chats := rg.Group("/chats")
	chatScoped := chats.Group("/:chatId")
	chatScoped.Use(resolveChatWorktree(c.app.Usecases.Worktree))
	chats.GET("/:chatId/blobs/:ref", chathandlers.NewBlobs(c.app.Usecases.AgentChat).Handle)
	rows := chathandlers.NewLedgerRows(c.app.Usecases.AgentChat)
	chats.GET("/:chatId/rows", rows.Page)
	chats.GET("/:chatId/rows/:seq", rows.One)

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
		c.app.Repositories,
		c.app.Repositories.ReviewThread,
		threadStream{c.app.Stream},
		c.app.Usecases.AgentChat,
		c.app.Usecases.AgentTurn,
		c.app.Usecases.AgentRunner,
		c.app.Usecases.AgentAnswer,
		c.app.Usecases.AgentProvider,
		c.app.Usecases.AgentChatFolder,
		chatWorktrees{app: c.app},
		c.app.Repositories.Placement,
	)
	worktreePkg.Register(
		repoScoped,
		c.app.Usecases.Workspace,
		c.app.Usecases.Workspace,
		c.app.GORM.Repositories,
		c.app.Operations,
		c.app.Usecases.AgentChat,
		c.eng.Git,
		c.app.Usecases.Worktree,
	)
	files.Register(
		chatScoped,
		c.app.Usecases.File,
	)
	documents.Register(rg, c.app.Usecases.File)
	git.Register(
		chatScoped,
		c.app.Usecases.Git,
		c.app.Operations,
	)
	terminal.Register(
		chatScoped,
		rg,
		c.eng.Terminal,
		c.app.GORM.TerminalProfiles,
	)
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
		c.app.GORM.Repositories,
		chatWorktrees{app: c.app},
		c.app.Repositories.Placement,
	)
	search.Register(
		chatScoped,
		c.eng.Search,
	)
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
	workspace.Register(
		repoScoped,
		c.app.Usecases.AgentChatFolder,
	)
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

func (c *Container) crowbarHome() (string, error) {
	return c.app.CrowbarHome(), nil
}
