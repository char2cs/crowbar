package usecases

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/app/usecases/git"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/discover"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/app/usecases/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// GORMStores carries the plain-CRUD stores the usecases compose. It mirrors the
// app-layer GORMStores struct but lives here so the usecases package never
// imports its parent package (which would be an import cycle).
type GORMStores struct {
	Projects                 store.Store[domain.Project, string]
	Repositories             store.Store[domain.Repository, string]
	TerminalProfiles         store.Store[domain.TerminalProfile, string]
	TerminalSessions         store.Store[domain.TerminalSession, string]
	AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]
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
	Worktree      worktree.Usecase
	BranchReview  branchreview.Usecase
	// TerminalMeta is the durable session metadata store implementation exposed
	// so the API layer can inject it into the terminal engine via SetMetaStore
	// after both the engine and the usecase are constructed.
	TerminalMeta engineterminal.SessionMetaStore
	// Agent is the agentic-chat usecase: spawning vendor CLIs as runners, moving
	// those runners between chats as their hooks report conversation changes, and
	// serving the /v0/agent REST surface.
	Agent *agent.Usecase
	// AgentWorkspaceReader is the SAME agent.WorkspaceReader (AgentChatsDir +
	// WorktreeDir) instance Agent was built with, exposed so the app layer can
	// wire the workspace-delete cascade's on-disk reap seam
	// (repositories.Container.ReapChatFiles) off the identical path resolution
	// PurgeChat already uses — without reimplementing it. It cannot be threaded
	// into repositories.New itself: the reader is built from repos.Workspace,
	// which does not exist until repositories.New returns.
	AgentWorkspaceReader agent.WorkspaceReader
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
	threadBroadcast agenttools.ThreadBroadcast,
) (*Container, error) {
	projectUsecase := project.New(
		gormStores.Projects,
		gormStores.Repositories,
	)
	workspaceUsecase := workspace.New(
		repos.Workspace,
		engines.Git,
		projectUsecase,
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
	projectImport := project.NewImport(project.ImportDeps{
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
	projectDelete := project.NewDelete(project.DeleteDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		CrowbarHome: crowbarHome,
	})
	worktreeUsecase := worktree.New(
		repos.Workspace,
		engines.Git,
		engines.Provider,
		gormStores.Repositories,
		nowFunc,
		crowbarHome,
		worktree.WithTerminalReaper(engines.Terminal),
	)
	branchReview := branchreview.New(
		repos.Workspace,
		repos.ReviewThread,
		gormStores.Repositories,
		engines.Git,
		nowFunc,
	)
	agentWSReader := &agentWorkspaceReader{
		workspaces:  repos.Workspace,
		repos:       gormStores.Repositories,
		crowbarHome: crowbarHome,
	}
	agentMinter, err := agenttools.NewTokenMinter()
	if err != nil {
		return nil, fmt.Errorf("usecases: new container: %w", err)
	}
	agentToolDeps, err := newAgentToolDeps(agentMinter, repos, branchReview, threadBroadcast)
	if err != nil {
		return nil, err
	}
	agentUsecase := agent.New(
		repos.AgentChat,
		repos.AgentRunner,
		engineagent.NewRegistry(),
		engines.Terminal,
		agentWSReader,
		gormStores.AgentProviderPreferences,
		crowbarHome,
		// nil probe → the usecase defaults to engineagent.Connected, the real
		// install probe. Only tests inject a stub to isolate from the host PATH.
		nil,
		agentMinter,
		agentToolDeps,
	)
	return &Container{
		Project:              projectUsecase,
		ProjectImport:        projectImport,
		ProjectDelete:        projectDelete,
		Workspace:            workspaceUsecase,
		File:                 fileUsecase,
		Git:                  gitUsecase,
		Terminal:             terminalUsecase,
		ProviderSync:         providerSync,
		Worktree:             worktreeUsecase,
		BranchReview:         branchReview,
		TerminalMeta:         terminalMeta,
		Agent:                agentUsecase,
		AgentWorkspaceReader: agentWSReader,
	}, nil
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
// GetBase/GetFiles/GetOutline with agenttools.ReviewReader's exact signatures, and
// reviewthread.ReviewThread already satisfies BOTH the read and the write half of
// the thread port, so the same repository is handed to Threads and ThreadWrites.
// The Idempotency map is built HERE, once, because it must outlive the per-request
// ToolSet for a retried post_review_comment to be recognized as a retry.
//
// threadBroadcast is injected from the app layer rather than derived here: fanning
// a thread out needs the wire DTO, and a usecase must not import the api layer's
// wire types. See agenttools.ThreadBroadcast.
//
// ChatLogs is deliberately NOT set here, unlike ChatReads: get_chat_log's ledger
// read (agenttools.ChatLogReader) is implemented on *agent.Usecase itself
// (agent.Usecase.ReadChatLog), which does not exist yet at this point in
// construction — the exact chicken-and-egg agent.New already resolves for
// Deps.Chats by assigning the usecase to itself once built. See its doc comment.
//
// Metrics is wired here too but, unlike every port above, is deliberately
// ABSENT from the refusal switch: agenttools.Metrics is the one fail-OPEN
// dependency in Deps — losing the call counters is never a reason to fail
// daemon startup or narrow the tool surface, so there is nothing to refuse.
func newAgentToolDeps(
	minter *agenttools.TokenMinter,
	repos *repositories.Container,
	review agenttools.ReviewReader,
	threadBroadcast agenttools.ThreadBroadcast,
) (agenttools.Deps, error) {
	switch {
	case minter == nil:
		return agenttools.Deps{}, fmt.Errorf("usecases: wire agent tools: no token minter")
	case repos.AgentRunner == nil:
		return agenttools.Deps{}, fmt.Errorf("usecases: wire agent tools: no runner store")
	case repos.AgentChat == nil:
		return agenttools.Deps{}, fmt.Errorf("usecases: wire agent tools: no chat store")
	case repos.Workspace == nil:
		return agenttools.Deps{}, fmt.Errorf("usecases: wire agent tools: no workspace store")
	case repos.ReviewThread == nil:
		return agenttools.Deps{}, fmt.Errorf("usecases: wire agent tools: no review thread store")
	case review == nil:
		return agenttools.Deps{}, fmt.Errorf("usecases: wire agent tools: no branch review usecase")
	case threadBroadcast == nil:
		return agenttools.Deps{}, fmt.Errorf("usecases: wire agent tools: no thread broadcaster")
	}
	chatReader := agentChatReader{chats: repos.AgentChat}
	return agenttools.Deps{
		Resolver: agenttools.NewResolver(
			minter,
			repos.AgentRunner,
			chatReader,
			repos.Workspace,
		),
		Review:          review,
		Threads:         repos.ReviewThread,
		ThreadWrites:    repos.ReviewThread,
		Idempotency:     agenttools.NewIdempotency(),
		ThreadBroadcast: threadBroadcast,
		ChatReads:       chatReader,
		Metrics:         agenttools.NewMetrics(),
	}, nil
}

// chatGetter is the minimal chat-read surface agentChatReader adapts.
type chatGetter interface {
	GetChat(ctx context.Context, id string) (domain.AgentChat, error)
	ListByWorkspace(ctx context.Context, wsID string) ([]domain.AgentChat, error)
}

// agentChatReader adapts the chat repository into agenttools.ChatReader. Only
// the name differs: the repository says GetChat because it also serves runners,
// while the tool surface's port is a plain Get.
type agentChatReader struct {
	chats chatGetter
}

// Get implements agenttools.ChatReader.
func (r agentChatReader) Get(
	ctx context.Context,
	chatID string,
) (domain.AgentChat, error) {
	return r.chats.GetChat(ctx, chatID)
}

// ListByWorkspace implements agenttools.ChatReader.
func (r agentChatReader) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.AgentChat, error) {
	return r.chats.ListByWorkspace(ctx, wsID)
}

// workspaceGetter is the minimal workspace-read surface agentWorkspaceReader
// needs: resolving the owning project/repo for a workspace id.
type workspaceGetter interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// repoGetter is the minimal repository-read surface agentWorkspaceReader needs to
// resolve a home-kind (adopted-checkout) workspace's on-disk identity slug from
// its repo id, mirroring worktree.resolveSlug's load-the-row pattern so the
// no-remote fallback can still reach the repo NAME.
type repoGetter interface {
	FindByKey(ctx context.Context, id string) (*domain.Repository, error)
}

// agentWorkspaceReader adapts the workspace repository into the agent
// usecase's WorkspaceReader seam (internal/app/usecases/agent.WorkspaceReader):
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

// WorktreeDir implements agent.WorkspaceReader.
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

// AgentChatsDir implements agent.WorkspaceReader: it resolves the directory that
// holds a workspace's agentic chat state, ALWAYS strictly under crowbar home.
//
// For a Crowbar-managed worktree (WorktreePath strictly under home) the chats dir
// is the sibling of the worktree (worktreepath.ChatsDir), reaped with the
// workspace root on delete. For an ADOPTED CHECKOUT — the repo-home / project-home
// whose WorktreePath is the user's REAL directory OUTSIDE home — the chats dir
// reroots under home at <home>/projects/<projectId>/<slug>/default/chats
// (worktreepath.HomeDefaultChatsDir), so a plaintext conversation ledger is never
// written onto the user's filesystem beside their repository (Task 7). The Cwd is
// unaffected: WorktreeDir still returns the adopted worktree unchanged.
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
// worktree.resolveSlug.
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
