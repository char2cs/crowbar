package usecases

import (
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/app/usecases/git"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/discover"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/app/usecases/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// GORMStores carries the plain-CRUD stores the usecases compose. It mirrors the
// app-layer GORMStores struct but lives here so the usecases package never
// imports its parent package (which would be an import cycle).
type GORMStores struct {
	Projects                 store.Store[domain.Project, string]
	Repositories             store.ScopedStore[domain.Repository, string]
	TerminalProfiles         store.Store[domain.TerminalProfile, string]
	TerminalSessions         store.Store[domain.TerminalSession, string]
	AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]
	AgentPermissionDefault   store.Store[domain.AgentPermissionDefault, string]
	AgentModelManifestFetch  store.Store[domain.AgentModelManifestFetch, string]
	AgentChatTelemetry       store.Store[domain.AgentChatTelemetry, string]
}

// Container holds every application usecase, composing the aggregate
// repositories, the GORM CRUD stores, and the engines.
type Container struct {
	Project       project.Usecase
	ProjectImport project.ImportUsecase
	ProjectDelete project.DeleteUsecase
	Workspace     workspace.Usecase
	File          file.Usecase
	Git           git.Usecase
	Terminal      terminal.Usecase
	ProviderSync  provider.Usecase
	BranchReview  branchreview.Usecase
	// TerminalMeta is the durable session metadata store implementation exposed
	// so the API layer can inject it into the terminal engine via SetMetaStore
	// after both the engine and the usecase are constructed.
	TerminalMeta engineterminal.SessionMetaStore
	// AgentChat is the chat aggregate — identity, title, model selection, hard
	// delete — and every read served off the conversation record it accumulates.
	AgentChat agentusecase.ChatUsecase
	// AgentTurn is the vendor CLI's hook ingress and everything it writes: turns,
	// tool calls, subagents, interruptions, streamed messages and telemetry.
	AgentTurn agentusecase.TurnUsecase
	// AgentRunner is the vendor CLI lifecycle: starting one on a chat, replacing
	// it, resuming it, stopping it, and delivering a React-authored prompt to it.
	AgentRunner agentusecase.RunnerUsecase
	// AgentAnswer is the human-in-the-loop answer desk: the hook relay blocked on
	// a person, and the Crowbar-side act of deciding for it.
	AgentAnswer agentusecase.AnswerUsecase
	// AgentProvider is the provider table and the MCP surface the vendor CLIs
	// call back into. It is global, never per workspace.
	AgentProvider agentusecase.ProviderUsecase
	// AgentChatFolder is the Chats panel's tree usecase: folder CRUD, chat
	// placement, and the cascading chat delete. It is built AFTER the chat usecase
	// because it holds it: erasing a chat and starting a CLI on one are that
	// usecase's job, and this one only decides which chats a delete takes.
	AgentChatFolder agentusecase.TreeUsecase
	// AgentWorkspaceReader is the SAME agentusecase.WorkspaceReader (AgentChatsDir +
	// WorktreeDir) instance the chat usecase was built with.
	AgentWorkspaceReader agentusecase.WorkspaceReader
	// Worktree resolves a chat to the workspace whose worktree it reads and
	// writes through (internal/app/usecases/worktree, spec
	// docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md §3): itself
	// first, then each ancestor in turn, nearest first. Built and wired here
	// (law 6) — every future chat-scoped handler declares its own narrow
	// Resolve(ctx, chatID) interface (law 4) and gets it satisfied by this
	// same value. Unused by any caller yet — spec §8 step 2.
	Worktree WorktreeResolver

	// agentToolMetrics is the SAME *agentusecase.ToolMetrics instance the agent tool
	// surface records through — held here only so AgentToolMetrics can read it
	// back out. It is buried inside agentusecase.ToolDeps otherwise, which is a
	// write-only counter: nothing in the daemon could reach the numbers.
	agentToolMetrics *agentusecase.ToolMetrics
}

// AgentToolMetrics reports how many times each agent tool was called and how
// many of those calls failed, over this daemon's lifetime.
//
// It exists because a counter nothing can read answers nothing: instrumenting
// this surface was a day-one requirement precisely so "do agents actually use
// these tools?" is settled by a number rather than an impression. The daemon
// logs the summary at shutdown (see app.Container.Shutdown).
//
// Deliberately NOT an HTTP route: these are a daemon-lifetime diagnostic, not a
// resource, and nothing in the product consumes them.
func (c *Container) AgentToolMetrics() map[string]agentusecase.ToolStat {
	return c.agentToolMetrics.Snapshot()
}

// New builds the usecases container. It takes the aggregate repositories, the
// GORM CRUD stores, and the engines rather than the app-layer GORMStores struct
// to keep the usecases package free of any dependency on its parent package.
// The agentic-chat usecase consumes the asynx-backed EventStore off the
// repositories container (repos.AgentChat); it no longer takes a broadcaster —
// agent-chat lifecycle frames are fanned out by the repository-layer hub
// projection (wired in repositories.Container), the single source of frames.
func New(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
	threadBroadcast agentusecase.ToolThreadBroadcast,
	opts ...Option,
) (*Container, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	projectUsecase := project.New(
		gormStores.Projects,
		gormStores.Repositories,
		repos.Workspace,
		repos.Placement,
		repos.Placement,
		// homeChats scopes a bare-root repo reorder's CHAT-kind Node siblings
		// to their project (see project.HomeChats).
		repos.AgentChat,
	)
	workspaceUsecase := workspace.New(
		repos.Workspace,
		engines.Git,
		projectUsecase,
		repos.Workspace,
		engines.Git,
		engines.Provider,
		gormStores.Repositories,
		nowFunc,
		crowbarHome,
		workspace.WithTerminalReaper(engines.Terminal),
		// Every workspace this usecase creates (fork, import, adopted repo
		// default) mints its own Node{Kind:workspace} row unconditionally at
		// creation — 2026-09-08 sidebar-placement-unification Task 7.
		workspace.WithNodes(repos.Placement),
	)
	fileUsecase := file.New(newFsEngineAdapter(engines.FS), workspaceUsecase, crowbarHome)
	gitUsecase := git.New(
		engines.Git,
		workspaceUsecase,
	)
	providerSync := provider.New(
		repos.Workspace,
		engines.Provider,
	)
	projectImport := newProjectImport(repos, gormStores, engines, crowbarHome, projectUsecase)
	projectDelete := project.NewDelete(project.DeleteDeps{
		Projects:       gormStores.Projects,
		Repos:          gormStores.Repositories,
		Workspaces:     repos.Workspace,
		RepoWorkspaces: workspaceUsecase,
		Nodes:          repos.Placement,
		CrowbarHome:    crowbarHome,
	})
	branchReview := branchreview.New(
		repos.Workspace,
		repos.ReviewThread,
		gormStores.Repositories,
		engines.Git,
		nowFunc,
	)
	agentic, err := newAgentWiring(repos, gormStores, engines, crowbarHome, branchReview, threadBroadcast, workspaceUsecase, o.chatSnapshots, o.browser)
	if err != nil {
		return nil, err
	}
	// Wired after agentic exists: the chat usecase depends on the workspace
	// usecase, so neither setter can be a workspace.New option.
	workspaceUsecase.SetChatObserver(agentic.chat)
	// Every workspace an import creates is minted under a chat; without this
	// both importers refuse (ErrNoOwningChats).
	owningChats := hierarchyOwningChats{tree: agentic.chatTree, chats: repos.AgentChat}
	workspaceUsecase.SetOwningChats(owningChats)
	projectImport.SetOwningChats(owningChats)
	// One chat→worktree resolver, the one the chat tree's delete already uses.
	worktreeUsecase := agentic.worktree
	// Built here: a terminal session is owned by a chat, and the resolver that
	// maps it to its worktree exists only once the chat usecase does.
	terminalMeta := terminal.NewSessionMetaStore(
		worktreeUsecase,
		gormStores.TerminalSessions,
		crowbarHome,
	)
	terminalUsecase := terminal.New(
		engines.Terminal,
		gormStores.TerminalProfiles,
		worktreeUsecase,
		terminalMeta,
	)
	return &Container{
		Project:       projectUsecase,
		ProjectImport: projectImport,
		ProjectDelete: projectDelete,
		Workspace:     workspaceUsecase,
		File:          fileUsecase,
		Git:           gitUsecase,
		Terminal:      terminalUsecase,
		ProviderSync:  providerSync,
		BranchReview:  branchReview,
		TerminalMeta:  terminalMeta,
		// Wrapped, not the raw chat usecase: a chat's ParentID/Order are
		// frozen at creation now (2026-09-08 sidebar-placement-unification
		// Task 5 for home-scoped, Task 8 for repo-scoped too) — every READ
		// needs its live Node position overlaid, or a chat filed into a
		// folder renders at the panel root forever, surviving a reload. See
		// agentusecase.NewHomeCorrectedChats' own doc for what this does and
		// does not close (no WS-broadcast fix, only the read/reload path).
		AgentChat:            agentusecase.NewHomeCorrectedChats(agentic.chat, repos.Placement),
		AgentTurn:            agentic.chat,
		AgentRunner:          agentic.chat,
		AgentAnswer:          agentic.chat,
		AgentProvider:        agentic.chat,
		AgentChatFolder:      agentic.chatTree,
		AgentWorkspaceReader: agentic.wsReader,
		Worktree:             worktreeUsecase,
		agentToolMetrics:     agentic.metrics,
	}, nil
}

// newAgentWiring assembles the agentic usecases in dependency order. It is split
// out of New only to keep that constructor within its length budget, mirroring
// newProjectImport; the wiring is otherwise unchanged.
//
// The order is not incidental: the Chats-panel tree usecase HOLDS the chat
// usecase, because deleting a chat there takes every chat threaded below it and
// erasing a chat — with the CLIs on it — is the chat usecase's job. The tree
// decides which chats go; it never learns how they are torn down.
// Option configures New.
type Option func(*options)

type options struct {
	chatSnapshots *agentusecase.ChatSnapshots
	browser       agentusecase.ToolBrowserBridge
}

// WithBrowserBridge gives agents the browser tools. Without it they are not
// registered, like any tool whose dependency is absent.
func WithBrowserBridge(b agentusecase.ToolBrowserBridge) Option {
	return func(o *options) { o.browser = b }
}

// WithChatSnapshots hands the chat usecase the snapshot owner the composition
// root built and fed to the repositories' watch seams. Without it the usecase
// builds a private owner no event reaches.
func WithChatSnapshots(s *agentusecase.ChatSnapshots) Option {
	return func(o *options) { o.chatSnapshots = s }
}

// newProjectImport assembles the project-import usecase's dependency set. It is
// split out of New only to keep that constructor within its length budget; the
// wiring is otherwise unchanged.
func newProjectImport(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
	homeSlots project.HomeSlots,
) project.ImportUsecase {
	return project.NewImport(project.ImportDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		Provider:    engines.Provider,
		Discover:    discover.Repos,
		RefRunner:   newRefRunner,
		Nodes:       repos.Placement,
		HomeSlots:   homeSlots,
		Now:         nowFunc,
		CrowbarHome: crowbarHome,
	})
}
