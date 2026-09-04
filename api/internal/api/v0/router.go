package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
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
// name: :projectId and :repoId are each defined exactly once by their group, so
// endpoints below them mount "/workspaces/:wsId/..."-relative paths without
// redefining the param.
//
// There is no dedicated /workspaces/:wsId sub-group any more: terminal was its
// only member and has moved to the flat /v0/chats/:chatId prefix below (spec
// §8 step 3). The remaining groups build their own "/workspaces/:wsId/..."
// paths off repoScoped, and follow terminal in later steps.
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
	// chatScoped is the flat /v0/chats/:chatId/... group spec §7.1 closes on:
	// no /projects/:projectId/repos/:repoId nesting, because chat ids are
	// globally unique and a consumer past creation never needs to resolve
	// ids it doesn't otherwise use. resolveChatWorktree is this group's own
	// scoping guard, the chat-scoped analogue of scopeWorkspaceToPath above:
	// it resolves :chatId to the workspace behind its worktree (spec §3,
	// c.app.Usecases.Worktree) and stashes it on the context
	// (reqscope.Workspace) so routes mounted here — terminal is the first,
	// spec §8 step 3 — read it back once per request instead of resolving it
	// per handler.
	chats := rg.Group("/chats")
	chatScoped := chats.Group("/:chatId")
	chatScoped.Use(resolveChatWorktree(c.app.Usecases.Worktree))
	// The per-CHAT lifecycle stream: the same agent-chat broadcaster the
	// repo-scoped .../chats/ws mount serves, scoped by agentChatDef's chatId
	// filter to the one chat named here.
	//
	// It is the chat-scoped replacement for watching ONE workspace's stream, and
	// it is load-bearing beyond the frames it carries. Subscribing to a single
	// workspace is what starts the daemon's provider poll — the GitHub/GitLab
	// PR-status detection that moves a branch to pr-open/pr-merged/pr-closed —
	// and the repo-wide list scope resolves no workspace, so it never did. This
	// mount resolves one through chatScoped's own resolveChatWorktree, which is
	// exactly what scopeWsID reads, so a client watching a chat starts the poll
	// for the worktree that chat holds. Without it, a frontend that stopped
	// watching .../workspaces/:wsId would leave every PR status frozen at `new`.
	chatScoped.GET("/ws", c.agentChats.Handle)

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
		c.app.Usecases.Workspace,
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
		// concerns + WS broadcaster chat.Register uses on the workspace-scoped
		// group); home.Register injects the resolved home :wsId so both scope
		// correctly. The chat-FOLDER surface is re-mounted with it, for the same
		// reason and then some: the home accumulates the most chats of any
		// workspace, so a home without folders is the one place the panel most
		// needs them and would not have them.
		c.app.Usecases.AgentChat,
		c.app.Usecases.AgentTurn,
		c.app.Usecases.AgentRunner,
		c.app.Usecases.AgentAnswer,
		c.app.Usecases.AgentProvider,
		c.app.Usecases.AgentChatFolder,
		c.app.Hub.BroadcastAgentChatFolder,
		c.agentChats.Handle,
		ws.DualServe,
	)
	workspaces.Register(
		repoScoped,
		c.app.Usecases.Workspace,
		c.app.Usecases.Workspace,
		c.app.GORM.Repositories,
		c.app.Repositories.Workspace,
		c.app.Repositories,
		c.eng.Git,
		c.app.Usecases.AgentChatFolder,
		c.app.Usecases.AgentChat,
		// The chat→worktree resolver (spec §3) behind the seven lifecycle verbs
		// this group also mounts on the chat prefix (spec §4.3). It is the SAME
		// value chatScoped's own middleware resolves through, so a verb reached
		// through .../chats/:id and a read reached through /chats/:chatId agree on
		// which worktree a chat is holding.
		c.app.Usecases.Worktree,
		c.app.Hub.BroadcastAgentChatFolder,
		c.workspaces.Handle,
		ws.DualServe,
	)
	// Files completes spec §4.2's SHARED bucket (§8 step 4): one worktree, one
	// tree, and every chat holding it reads and writes the same files. It mounts
	// on BOTH groups for now — the chat prefix is where it lives, and the
	// workspace prefix is simply not retired until §8 step 6 — and needs no
	// workspace reader on either, because each group resolves the worktree
	// before the handlers run. The home group above keeps its own /home/files
	// surface, for the project-level row no chat resolves to.
	files.Register(
		repoScoped,
		chatScoped,
		c.app.Usecases.File,
		c.files.Handle,
	)
	// Git is the first of spec §4.2's SHARED bucket to move (§8 step 4): one
	// worktree, one answer, and every chat holding it sees the same writes. It
	// mounts on BOTH groups for now — the chat prefix is where it lives, and
	// the workspace prefix is simply not retired until §8 step 6 — and needs no
	// workspace reader on either, because each group resolves the worktree
	// before the handlers run.
	git.Register(
		repoScoped,
		chatScoped,
		c.app.Usecases.Git,
		c.app.Repositories.Workspace,
		c.app.Repositories,
		c.git.Handle,
		ws.DualServe,
	)
	// Terminal is the first group to move onto the flat chat prefix (spec §8
	// step 3): /v0/chats/:chatId/terminals[...]. It needs no workspace reader —
	// chatScoped's resolveChatWorktree already resolved one onto the request
	// context for the PTY's CWD.
	terminal.Register(
		chatScoped,
		rg,
		c.eng.Terminal,
		c.app.GORM.TerminalProfiles,
		c.terminals,
		c.terminals.Handle,
		ws.DualServe,
	)
	// The agentic-chat REST + WS surface is repo-scoped (Task 17): a chat's
	// workspace is optional and mutable, so its routes mount on repoScoped
	// (.../repos/:repoId) rather than wsScoped — no chat route names a
	// workspace any more. Handlers that once trusted :wsId now either read
	// :repoId or resolve a specific chat's own workspace from the chat itself
	// (GetChat), never from the URL. rg carries the GLOBAL
	// provider-preferences write route (/settings/chat/providers), mounted
	// once outside the entity hierarchy like /settings/terminal/profiles.
	chat.Register(
		repoScoped,
		rg,
		c.app.Usecases.AgentChat,
		c.app.Usecases.AgentTurn,
		c.app.Usecases.AgentRunner,
		c.app.Usecases.AgentAnswer,
		c.app.Usecases.AgentProvider,
		c.app.Usecases.AgentChatFolder,
		// Read by POST /chats alone, and only when its body asks to IMPORT a
		// branch: the create needs the repo's on-disk path and remote to describe
		// the branch it is adopting (spec §4.1).
		c.app.GORM.Repositories,
		// The git fields a worktree-owning chat carries on its own DTO (spec §5),
		// so ONE read of the chat list answers everything the workspace list used
		// to. The home group deliberately mounts these handlers WITHOUT it: the
		// project home is a bare project-level row with no repo and no git surface
		// at all, so there is no worktree there to describe.
		chatWorktrees{app: c.app},
		c.app.Hub.BroadcastAgentChatFolder,
		c.agentChats.Handle,
	)
	// Search, review, and identity are the rest of spec §4.2's SHARED bucket
	// (§8 step 4c): one worktree, one answer, and every chat holding it sees
	// the same reads. Each mounts on BOTH groups for now — the chat prefix is
	// where it lives, and the workspace prefix is simply not retired until §8
	// step 6 — and needs no workspace reader on the chat mount, because
	// chatScoped's resolveChatWorktree middleware has already resolved the
	// worktree onto the request context.
	search.Register(
		repoScoped,
		chatScoped,
		c.eng.Search,
		c.app.Repositories.Workspace,
	)
	// Provider is the second group of spec §4.2's OWNED bucket to move (§8
	// step 5): the poll answers per chat's resolved worktree, and the session
	// itself is never shared with a sibling. It mounts on BOTH groups for now
	// — the chat prefix is where the State route lives, and the workspace
	// prefix is simply not retired until §8 step 6 — while /protected-branches
	// stays exactly where it was: it is repo-level, not worktree-owned, and
	// does not move.
	provider.Register(
		repoScoped,
		chatScoped,
		c.eng.Provider,
		c.app.Repositories.Workspace,
	)
	review.Register(
		repoScoped,
		chatScoped,
		c.app.Usecases.BranchReview,
	)
	threads.Register(
		repoScoped,
		c.app.Repositories.ReviewThread,
		c.threads,
		c.threads.Handle,
		ws.DualServe,
	)
	// Editor/LSP completes spec §4.2's OWNED bucket (§8 step 5): the resolver
	// still runs, for a CWD, but the LSP session itself is never shared with a
	// sibling chat holding the same worktree (spec law 5). It mounts on BOTH
	// groups for now — the chat prefix is where it lives, and the workspace
	// prefix is simply not retired until §8 step 6 — and needs no workspace
	// reader on the chat mount, because chatScoped's resolveChatWorktree
	// middleware has already resolved the worktree onto the request context.
	editor.Register(
		repoScoped,
		chatScoped,
		c.eng.LSP,
		c.eng.Git,
		c.app.Repositories.Workspace,
		c.lsp.Handle,
	)
	identity.Register(
		repoScoped,
		chatScoped,
		c.eng.Identity,
		c.app.Repositories.Workspace,
	)
}
