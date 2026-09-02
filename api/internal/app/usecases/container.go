package usecases

import (
	"context"
	"fmt"

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
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
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
	// WorktreeDir) instance the chat usecase was built with, exposed so the app layer can
	// wire the workspace-delete cascade's on-disk reap seam
	// (repositories.Container.ReapChatFiles) off the identical path resolution
	// PurgeChat already uses — without reimplementing it. It cannot be threaded
	// into repositories.New itself: the reader is built from repos.Workspace,
	// which does not exist until repositories.New returns.
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

// WorktreeResolver is the Resolve(ctx, chatID) shape spec §3
// (docs/superpowers/specs/2026-09-02-chat-scoped-api-design.md) describes a
// future handler calling in place of h.wsReader.Get(ctx, wsID) — h.resolver
// .Resolve(ctx, chatID) instead. The container's own worktreeResolver is the
// one concrete value that satisfies it.
type WorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

// worktreeResolver adapts the package-level worktree.Resolve function
// (internal/app/usecases/worktree) into a WorktreeResolver value: chats and
// workspaces are the container's own concrete usecases, satisfying the
// resolver's two locally-declared ports (law 4) structurally.
type worktreeResolver struct {
	chats      worktree.ChatAncestryReader
	workspaces worktree.WorkspaceReader
}

// Resolve implements WorktreeResolver.
func (r worktreeResolver) Resolve(
	ctx context.Context,
	chatID string,
) (domain.Workspace, error) {
	return worktree.Resolve(ctx, chatID, r.chats, r.workspaces)
}

// chatAncestrySource is the minimal chat-read surface chatAncestryReader
// composes. GetChat reads one full row; Ancestors (agentusecase.Usecase.Ancestors)
// returns parent ids only, nearest first, EXCLUDING the subject — "what a
// thread inherits," not "this chat's own worktree."
type chatAncestrySource interface {
	GetChat(ctx context.Context, id string) (domain.Chat, error)
	Ancestors(ctx context.Context, chatID string) ([]string, error)
}

// chatAncestryReader adapts the chat usecase into worktree.ChatAncestryReader
// (internal/app/usecases/worktree): Resolve needs the subject chat's own
// WorkspaceID, to short-circuit a chat that owns its own worktree, which the
// chat usecase's Ancestors alone can't give it — so this composes GetChat for
// the subject with Ancestors for everything above it, subject first, each
// parent then resolved to its own full row in turn.
type chatAncestryReader struct {
	chats chatAncestrySource
}

// Ancestors implements worktree.ChatAncestryReader.
func (r chatAncestryReader) Ancestors(
	ctx context.Context,
	chatID string,
) ([]domain.Chat, error) {
	self, err := r.chats.GetChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("chat ancestry: chat %s: %w", chatID, err)
	}
	parentIDs, err := r.chats.Ancestors(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("chat ancestry: ancestors of %s: %w", chatID, err)
	}
	chats := make([]domain.Chat, 0, len(parentIDs)+1)
	chats = append(chats, self)
	for _, id := range parentIDs {
		parent, err := r.chats.GetChat(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("chat ancestry: chat %s: %w", id, err)
		}
		chats = append(chats, parent)
	}
	return chats, nil
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
) (*Container, error) {
	projectUsecase := project.New(
		gormStores.Projects,
		gormStores.Repositories,
		repos.Workspace,
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
	)
	fileUsecase := file.New(
		newFsEngineAdapter(engines.FS),
		workspaceUsecase,
	)
	gitUsecase := git.New(
		engines.Git,
		workspaceUsecase,
	)
	terminalMeta := terminal.NewSessionMetaStore(
		repos.Workspace,
		gormStores.TerminalSessions,
		crowbarHome,
	)
	terminalUsecase := terminal.New(
		engines.Terminal,
		gormStores.TerminalProfiles,
		repos.Workspace,
		terminalMeta,
	)
	providerSync := provider.New(
		repos.Workspace,
		engines.Provider,
	)
	projectImport := newProjectImport(repos, gormStores, engines, crowbarHome)
	projectDelete := project.NewDelete(project.DeleteDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		CrowbarHome: crowbarHome,
	})
	branchReview := branchreview.New(
		repos.Workspace,
		repos.ReviewThread,
		gormStores.Repositories,
		engines.Git,
		nowFunc,
	)
	agentic, err := newAgentWiring(repos, gormStores, engines, crowbarHome, branchReview, threadBroadcast, workspaceUsecase)
	if err != nil {
		return nil, err
	}
	// Wired AFTER agentic exists, not as a workspace.New(...) option: the chat
	// usecase itself depends on the workspace usecase (Promote forks a
	// workspace through worktreeChildCreator above), so at workspace.New's own
	// call site the chat usecase does not exist yet to hand over. See
	// workspace.Usecase.SetChatObserver's doc comment.
	workspaceUsecase.SetChatObserver(agentic.chat)
	// Both ways a branch becomes locked at RUNTIME — the user's own lock, and a
	// provider poll reporting the branch protected — hand the workspace to the
	// same reconciler, so its owning row is a branch row from that instant
	// rather than from the next boot's backfill. Wired here for the same reason
	// as the observer above: the chat tree is built after both of them.
	workspaceUsecase.SetOwningChatReconciler(agentic.chatTree)
	providerSync.SetOwningChatReconciler(agentic.chatTree)
	// Every workspace the import paths create is minted UNDER a chat from here
	// on. Wired at the same point and for the same reason as the two setters
	// above — the chat tree does not exist until agentic is built — and this is
	// the wiring that makes the §0 orphan unrepresentable rather than merely
	// reconciled: without it both importers refuse outright (ErrNoOwningChats)
	// instead of falling back to a workspace-first create.
	owningChats := hierarchyOwningChats{tree: agentic.chatTree}
	workspaceUsecase.SetOwningChats(owningChats)
	projectImport.SetOwningChats(owningChats)
	worktreeUsecase := worktreeResolver{
		chats:      chatAncestryReader{chats: agentic.chat},
		workspaces: workspaceUsecase,
	}
	return &Container{
		Project:              projectUsecase,
		ProjectImport:        projectImport,
		ProjectDelete:        projectDelete,
		Workspace:            workspaceUsecase,
		File:                 fileUsecase,
		Git:                  gitUsecase,
		Terminal:             terminalUsecase,
		ProviderSync:         providerSync,
		BranchReview:         branchReview,
		TerminalMeta:         terminalMeta,
		AgentChat:            agentic.chat,
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

// agentWiring is the agentic surface as one value: the chat usecase, the Chats
// panel's tree usecase built on top of it, and the two handles the container has
// to keep hold of beside them — the workspace reader the app layer reuses for the
// delete cascade's on-disk reap, and the tool metrics nothing else can reach.
type agentWiring struct {
	chat     *agentusecase.Usecase
	chatTree agentusecase.TreeUsecase
	wsReader agentusecase.WorkspaceReader
	metrics  *agentusecase.ToolMetrics
}

// newAgentWiring assembles the agentic usecases in dependency order. It is split
// out of New only to keep that constructor within its length budget, mirroring
// newProjectImport; the wiring is otherwise unchanged.
//
// The order is not incidental: the Chats-panel tree usecase HOLDS the chat
// usecase, because deleting a chat there takes every chat threaded below it and
// erasing a chat — with the CLIs on it — is the chat usecase's job. The tree
// decides which chats go; it never learns how they are torn down.
func newAgentWiring(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
	review agentusecase.ToolReviewReader,
	threadBroadcast agentusecase.ToolThreadBroadcast,
	workspaceUsecase workspace.Usecase,
) (agentWiring, error) {
	wsReader := &agentWorkspaceReader{
		workspaces:  repos.Workspace,
		repos:       gormStores.Repositories,
		crowbarHome: crowbarHome,
	}
	minter, err := agentusecase.NewTokenMinter()
	if err != nil {
		return agentWiring{}, fmt.Errorf("usecases: new container: %w", err)
	}
	// The Chats-panel lineage read, built FIRST and from the chat repository
	// directly — folder rows and conversation rows are one table now, so one
	// store answers it. The spawn path needs it to tell a thread which chats it
	// reads, and it is deliberately not taken off the tree usecase, which already
	// holds the chat usecase for the delete cascade and would close a
	// construction cycle if that usecase reached back into it. (The tool surface
	// needs the same answer and gets it from the chat usecase, which re-exposes
	// this as Ancestors.)
	lineage := agentusecase.NewChatLineage(repos.AgentChat)
	toolDeps, err := newAgentToolDeps(minter, repos, review, threadBroadcast, workspaceUsecase)
	if err != nil {
		return agentWiring{}, err
	}
	chat := agentusecase.New(agentusecase.Deps{
		Chats:           repos.AgentChat,
		Runners:         repos.AgentRunner,
		Activity:        repos.AgentActivity,
		Agents:          engines.Agents,
		Terminal:        engines.Terminal,
		Workspace:       wsReader,
		Worktree:        worktreeChildCreator{worktree: workspaceUsecase},
		Lineage:         lineage,
		ProviderPrefs:   gormStores.AgentProviderPreferences,
		PermissionPrefs: gormStores.AgentPermissionDefault,
		Home:            crowbarHome,
		// Installed is left nil: the usecase defaults to Agent.Installed, the real
		// install probe. Only tests inject a stub to isolate from the host PATH.
		Minter: minter,
		Tools:  toolDeps,
	})
	chatTree := agentusecase.NewTree(
		repos.AgentChat,
		chat,
		chat.Work(),
		workspaceGitStatusReader{workspace: workspaceUsecase},
		repos.Workspace,
	)
	return agentWiring{
		chat:     chat,
		chatTree: chatTree,
		wsReader: wsReader,
		metrics:  toolDeps.Metrics,
	}, nil
}

// newProjectImport assembles the project-import usecase's dependency set. It is
// split out of New only to keep that constructor within its length budget; the
// wiring is otherwise unchanged.
func newProjectImport(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
) project.ImportUsecase {
	return project.NewImport(project.ImportDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		Provider:    engines.Provider,
		Discover:    discover.Repos,
		RefRunner:   newRefRunner,
		Now:         nowFunc,
		CrowbarHome: crowbarHome,
	})
}

// newAgentToolDeps assembles the production agent capability surface and REFUSES
// to return a partial one.
//
// agenttools registers no tool whose dependency is missing, which is the right
// default for the package but a terrible one to discover in production: a daemon
// wired with a nil port would boot clean, serve chats normally, and hand every
// agent an empty tool list with nothing anywhere reporting why. Checking here
// turns that silent degradation into a failed start.
// The review ports need no adapter: branchreview.Usecase already has
// GetScope/GetOutline with agentusecase.ToolReviewReader's exact signatures, and
// reviewthread.ReviewThread already satisfies BOTH the read and the write half of
// the thread port, so the same repository is handed to Threads and ThreadWrites.
// The Idempotency map is built HERE, once, because it must outlive the per-request
// ToolSet for a retried post_review_comment to be recognized as a retry.
//
// threadBroadcast is injected from the app layer rather than derived here: fanning
// a thread out needs the wire DTO, and a usecase must not import the api layer's
// wire types. See agentusecase.ToolThreadBroadcast.
//
// workspaces is the workspace usecase itself, which already satisfies
// set_branch_name's narrow WorkspaceBranchRenamer port by name. It is a
// parameter rather than something read off repos because renaming a branch is a
// USECASE verb — a git ref rename plus one record write, with the locked/held
// refusals in front of it — not a store write. It is in the refusal switch for
// the reason every other port is: a nil here withdraws set_branch_name, and an
// agent that cannot name its own branch leaves every generated provisional name
// in place with nothing anywhere reporting why.
//
// ChatLogs is deliberately NOT set here, unlike ChatReads: get_chat_log's ledger
// read (agentusecase.ToolChatLogReader) is implemented by the agent CHAT concern
// (agentusecase.ChatUsecase.ReadChatLog), which does not exist yet at this point in
// construction — the exact chicken-and-egg agentusecase.New already resolves for
// Deps.Chats, Deps.ChatLogs and Deps.Lineage by binding the usecase to itself once
// built. See its doc comment.
//
// Metrics is wired here too but, unlike every port above, is deliberately
// ABSENT from the refusal switch: agentusecase.ToolMetrics is the one fail-OPEN
// dependency in Deps — losing the call counters is never a reason to fail
// daemon startup or narrow the tool surface, so there is nothing to refuse.
func newAgentToolDeps(
	minter *agentusecase.TokenMinter,
	repos *repositories.Container,
	review agentusecase.ToolReviewReader,
	threadBroadcast agentusecase.ToolThreadBroadcast,
	workspaces agentusecase.ToolWorkspaceBranchRenamer,
) (agentusecase.ToolDeps, error) {
	switch {
	case minter == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no token minter")
	case workspaces == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no workspace usecase")
	case repos.AgentRunner == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no runner store")
	case repos.AgentChat == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no chat store")
	case repos.Workspace == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no workspace store")
	case repos.ReviewThread == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no review thread store")
	case review == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no branch review usecase")
	case threadBroadcast == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no thread broadcaster")
	}
	chatReader := agentChatReader{chats: repos.AgentChat}
	return agentusecase.ToolDeps{
		Resolver: agentusecase.NewToolResolver(
			minter,
			repos.AgentRunner,
			chatReader,
			repos.Workspace,
		),
		Workspaces:      workspaces,
		Review:          review,
		Threads:         repos.ReviewThread,
		ThreadWrites:    repos.ReviewThread,
		Idempotency:     agentusecase.NewToolIdempotency(),
		ThreadBroadcast: threadBroadcast,
		ChatReads:       chatReader,
		Metrics:         agentusecase.NewToolMetrics(),
	}, nil
}

// chatGetter is the minimal chat-read surface agentChatReader adapts.
type chatGetter interface {
	GetChat(ctx context.Context, id string) (domain.Chat, error)
	ListChats(ctx context.Context) ([]domain.Chat, error)
}

// agentChatReader adapts the chat repository into agentusecase.ToolChatReader. Only
// the name differs: the repository says GetChat because it also serves runners,
// while the tool surface's port is a plain Get.
type agentChatReader struct {
	chats chatGetter
}

// Get implements agentusecase.ToolChatReader.
func (r agentChatReader) Get(
	ctx context.Context,
	chatID string,
) (domain.Chat, error) {
	return r.chats.GetChat(ctx, chatID)
}

// ListChats implements agentusecase.ToolChatReader.
func (r agentChatReader) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return r.chats.ListChats(ctx)
}

// workspaceGitStatusReader adapts the workspace usecase into the chat tree
// usecase's WorkspaceGitStatus seam (internal/app/usecases/chat/internal/tree.
// WorkspaceGitStatus): DeletePreview needs each workspace-owning row's file
// counts, and Get's Added/Deleted are the SAME already-synced numbers the
// sidebar itself renders — never a live git call recomputed per row on every
// preview.
type workspaceGitStatusReader struct {
	workspace workspace.Usecase
}

// WorkingTreeSummary implements agentusecase.TreeWorkspaceGitStatus.
func (w workspaceGitStatusReader) WorkingTreeSummary(
	ctx context.Context,
	workspaceID string,
) (int, int, error) {
	ws, err := w.workspace.Get(ctx, workspaceID)
	if err != nil {
		return 0, 0, err
	}
	return ws.Added, ws.Deleted, nil
}

// worktreeChildCreator adapts the worktree hierarchy usecase into the agent
// usecase's WorktreeCreator seam (internal/app/usecases/chat.WorktreeCreator):
// Promote names only the fork parent it forks from, and this fills in the rest
// of workspace.CreateChildInput the way every other spontaneous create does —
// leaving RepoID/RepoPath/RemoteURL/ParentBranch blank so CreateChild's own
// parent-inherited defaulting resolves them, and Branch blank so it generates
// and collision-checks a provisional name (model spec §4.1).
//
// OwnWorktree is the one field NOT left to that defaulting: resolveInherited's
// default is "inherit whether the PARENT owns a worktree" (model spec §4.1's
// taxonomy rule for an ordinary create), but promotion's entire point is
// giving the chat a worktree of its own — a fork parent that is itself a
// workspace-less bubble must not silently promote this chat into another
// bubble. So it is forced true here, always.
type worktreeChildCreator struct {
	worktree workspace.Usecase
}

// CreateChildWorkspace implements agentusecase.WorktreeCreator.
func (w worktreeChildCreator) CreateChildWorkspace(
	ctx context.Context,
	forkParentID string,
) (domain.Workspace, error) {
	ownWorktree := true
	return w.worktree.CreateChild(ctx, workspace.CreateChildInput{
		ParentID:    forkParentID,
		OwnWorktree: &ownWorktree,
	})
}

// CreateImportedWorkspace implements agentusecase.WorktreeCreator. Unlike
// CreateChildWorkspace above it fills in NOTHING: an import already knows its
// repo, its branch and its git-lineage parent, because the caller discovered
// the branch in that repository and resolved its PR base before asking. Leaving
// any of it to CreateChild's parent-inherited defaulting would resolve it from
// a parent workspace an import rooted at the repo does not have.
//
// OwnWorktree is forced true for the same reason it is forced above: the
// taxonomy default is "inherit whether the parent owns a worktree", and an
// import whose lineage parent happens to be a bubble must still get a real
// worktree of its own — that is the entire request.
func (w worktreeChildCreator) CreateImportedWorkspace(
	ctx context.Context,
	spec agentusecase.ImportSpec,
) (domain.Workspace, error) {
	ownWorktree := true
	return w.worktree.CreateChild(ctx, workspace.CreateChildInput{
		RepoID:       spec.RepoID,
		ProjectID:    spec.ProjectID,
		RepoPath:     spec.RepoPath,
		RemoteURL:    spec.RemoteURL,
		Branch:       spec.Branch,
		ParentID:     spec.ParentWorkspaceID,
		ParentBranch: spec.ParentBranch,
		ForceLocked:  spec.ForceLocked,
		OwnWorktree:  &ownWorktree,
	})
}

// DiscardChildWorkspace implements agentusecase.WorktreeCreator: it removes a
// workspace a promotion minted and then could not finish, through the SAME
// cascade a user-initiated workspace removal takes, so the worktree and the
// branch go together and nothing is left half-reaped. The cascade's own guards
// still apply — by the time this runs the chat has stopped pointing at the
// workspace, so its working-chat guard sees an empty workspace and passes.
func (w worktreeChildCreator) DiscardChildWorkspace(
	ctx context.Context,
	workspaceID string,
) error {
	return w.worktree.DeleteCascade(ctx, workspaceID)
}

// hierarchyOwningChats adapts the Chats-panel tree usecase into the worktree
// hierarchy's OwningChats seam (usecases/workspace.OwningChats) — the mirror of
// worktreeChildCreator above, pointing the other way: that one lets a chat ask
// for a worktree, this one makes every worktree the import path creates be born
// under a chat.
//
// Three of the four verbs are the tree's own, unchanged. Only the import
// differs, and only in shape: the hierarchy describes a branch in its OWN
// vocabulary (workspace.ImportedBranch, which it declares because it is the
// consumer) and wants back the one thing its chain walk records — the workspace
// id — while the tree speaks its own ImportSpec and hands back both ids. This
// is exactly the translation the container exists to do, and it is why neither
// usecase has to import the other.
//
// The delegation is written out rather than embedded on purpose: an embedded
// TreeUsecase would silently expose a SECOND ImportBranchAsChat with a
// different signature, and which one a caller reached would depend on where it
// was standing.
type hierarchyOwningChats struct {
	tree agentusecase.TreeUsecase
}

// ImportBranchAsChat implements workspace.OwningChats.
func (h hierarchyOwningChats) ImportBranchAsChat(
	ctx context.Context,
	in workspace.ImportedBranch,
) (string, error) {
	_, workspaceID, err := h.tree.ImportBranchAsChat(ctx, agentusecase.ImportSpec{
		RepoID:            in.RepoID,
		ProjectID:         in.ProjectID,
		RepoPath:          in.RepoPath,
		RemoteURL:         in.RemoteURL,
		Branch:            in.Branch,
		ParentWorkspaceID: in.ParentWorkspaceID,
		ParentBranch:      in.ParentBranch,
		ForceLocked:       in.ForceLocked,
	})
	return workspaceID, err
}

// MintOwningChat implements workspace.OwningChats.
func (h hierarchyOwningChats) MintOwningChat(
	ctx context.Context,
	parentWorkspaceID string,
) (string, error) {
	return h.tree.MintOwningChat(ctx, parentWorkspaceID)
}

// AttachOwningWorkspace implements workspace.OwningChats.
func (h hierarchyOwningChats) AttachOwningWorkspace(
	ctx context.Context,
	chatID string,
	ws domain.Workspace,
) error {
	return h.tree.AttachOwningWorkspace(ctx, chatID, ws)
}

// DiscardOwningChat implements workspace.OwningChats.
func (h hierarchyOwningChats) DiscardOwningChat(
	ctx context.Context,
	chatID string,
) error {
	return h.tree.DiscardOwningChat(ctx, chatID)
}

// workspaceGetter is the minimal workspace-read surface agentWorkspaceReader
// needs: resolving the owning project/repo for a workspace id.
type workspaceGetter interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// repoGetter is the minimal repository-read surface agentWorkspaceReader needs to
// resolve a home-kind (adopted-checkout) workspace's on-disk identity slug from
// its repo id, mirroring workspace's worktree hierarchy resolveSlug's load-the-row pattern so the
// no-remote fallback can still reach the repo NAME.
type repoGetter interface {
	FindByKey(ctx context.Context, id string) (*domain.Repository, error)
}

// agentWorkspaceReader adapts the workspace repository into the agent
// usecase's WorkspaceReader seam (internal/app/usecases/chat.WorkspaceReader):
// given a workspace id, it resolves the owning project/repo and the git
// worktree directory from the workspace read model's stored WorktreePath
// (WorktreeDir), and the directory holding the workspace's agentic chat state
// (AgentChatsDir), which for an adopted checkout reroots under crowbar home.
//
// It shares the container's injected crowbarHome resolver (the same one every
// other path-deriving usecase here is built with) rather than reading
// metadata.GetHomePath() directly: path-deriving usecases MUST resolve against
// the home the adapter container was actually opened against (see
// adapter.Container.CrowbarHome's doc comment), and a hermetic test that
// overrides the home via adapter.WithHomeDir (rather than the CROWBAR_HOME env
// var) would otherwise silently diverge.
type agentWorkspaceReader struct {
	workspaces  workspaceGetter
	repos       repoGetter
	crowbarHome func() (string, error)
}

// WorktreeDir implements agentusecase.WorkspaceReader.
func (r *agentWorkspaceReader) WorktreeDir(
	ctx context.Context,
	workspaceID string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	home, err := r.crowbarHome()
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: crowbar home: %w", err)
	}
	w, err := r.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: get workspace: %w", err)
	}
	return home, w.ProjectID, w.RepoID, w.WorktreePath, nil
}

// AgentChatsDir implements agentusecase.WorkspaceReader: it resolves the directory that
// holds a workspace's OWN agent-work state (per-spawn tmp dirs, the per-runner
// hook-delivery journal), ALWAYS strictly under crowbar home. It is NOT where a
// chat's own ledger lives — that is worktreepath.LedgerChatsDir, keyed by the
// chat's id alone, because WorkspaceID is optional and mutable (spec §1.5) and
// this lookup requires a resolvable workspace.
//
// For a Crowbar-managed worktree (WorktreePath strictly under home) the chats dir
// is the sibling of the worktree (worktreepath.ChatsDir), reaped with the
// workspace root on delete. For an ADOPTED CHECKOUT — the repo-home / project-home
// whose WorktreePath is the user's REAL directory OUTSIDE home — the chats dir
// reroots under home at <home>/projects/<projectId>/<slug>/default/chats
// (worktreepath.HomeDefaultChatsDir), so plaintext state is never written onto
// the user's filesystem beside their repository. The Cwd is unaffected:
// WorktreeDir still returns the adopted worktree unchanged.
//
// The discriminator is the under-home test, NOT the workspace Kind: the
// chat-hosting repo-home is a Kind=git / IsDefault workspace (adoptRepoHome does
// not set WorkspaceKindHome) whose WorktreePath is repo.Path outside home, so
// keying on Kind would both MISS it (leaving the real leak) and break the
// project-level home (WorkspaceKindHome, but with no repo id to resolve a slug
// from). Keying on the path that must never be written to is what makes the output
// provably safe for every kind.
func (r *agentWorkspaceReader) AgentChatsDir(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	home, err := r.crowbarHome()
	if err != nil {
		return "", fmt.Errorf("usecases: agent workspace reader: crowbar home: %w", err)
	}
	w, err := r.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("usecases: agent workspace reader: get workspace: %w", err)
	}
	if worktreepath.UnderHome(w.WorktreePath, home) {
		return worktreepath.ChatsDir(w.WorktreePath), nil
	}
	slug, err := r.repoSlug(ctx, w.RepoID)
	if err != nil {
		return "", err
	}
	chatsDir := worktreepath.HomeDefaultChatsDir(home, w.ProjectID, slug)
	// Fail closed if the rerooted path escapes home. filepath.Join (inside
	// HomeDefaultChatsDir) cleans "..", so a crafted repo remote whose RemoteSlug
	// contains "../" segments could otherwise resolve OUTSIDE crowbar home — which
	// would let a plaintext ledger be WRITTEN onto the user's real filesystem. Real
	// remotes never do this; a poisoned one gets an error, not an escape (the
	// removal sites re-assert the same invariant as a second backstop).
	if !worktreepath.UnderHome(chatsDir, home) {
		return "", fmt.Errorf(
			"usecases: agent workspace reader: resolved chats dir %q escapes crowbar home %q (poisoned slug %q)",
			chatsDir, home, slug,
		)
	}
	return chatsDir, nil
}

// repoSlug resolves the repo's on-disk identity slug for an adopted-checkout
// workspace's rerooted chats dir. A blank repo id (the project-level home has no
// repo) yields an empty slug, which HomeDefaultChatsDir collapses to the project
// directory — still strictly under home. It always loads the repo row so the
// no-remote / unparseable-URL fallback can reach the repo NAME, mirroring
// workspace's worktree hierarchy resolveSlug.
func (r *agentWorkspaceReader) repoSlug(
	ctx context.Context,
	repoID string,
) (string, error) {
	if repoID == "" {
		return "", nil
	}
	repo, err := r.repos.FindByKey(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("usecases: agent workspace reader: get repo: %w", err)
	}
	if repo == nil {
		return "", fmt.Errorf("usecases: agent workspace reader: repo %q not found", repoID)
	}
	return worktreepath.RemoteSlug(*repo), nil
}
