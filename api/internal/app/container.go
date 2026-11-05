package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/app/stream/resources"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the application layer: the aggregate repositories, the
// GORM CRUD stores, the composed usecases, and the stream with the resources
// its held scopes run.
type Container struct {
	Repositories *repositories.Container
	GORM         *GORMStores
	Usecases     *usecases.Container
	// Operations runs the work routes hand off after answering 202.
	Operations *operation.Service
	// RepoPlacements is the one position read every repo list goes through, so
	// a project whose tree cannot be read is logged once per boot.
	RepoPlacements *dto.Placements
	// Stream serves every client the scopes it holds.
	Stream *stream.Hub
	// resources are what a held scope runs; Close stops what is still running.
	resources   *resources.Set
	crowbarHome string
	// Browser carries browser tool calls to the desktop app's tabs.
	Browser *browserbridge.Bridge

	// engines lets Shutdown quiesce the terminal engine (kill every PTY, join the exit
	// callbacks) before draining the aggregates below, which those callbacks write into.
	engines *engine.Container

	// axWorkspace, axReviewThread and axAgentChat are the per-type asynx
	// singletons, kept so the ordered graceful shutdown can drain each.
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.Chat]
	// axTree is the per-project placement aggregate's singleton.
	axTree asynx.Asynx[tree.Tree]
}

// New constructs the application layer from the engine and adapter containers,
// and runs post's steps.
func New(
	ctx context.Context,
	engines *engine.Container,
	adapters *adapter.Container,
	post migrate.PostOpenRunner,
	encode stream.Encoder,
	newSidebar func(*Container) stream.Sidebar,
) (*Container, error) {
	rm, err := repositories.OpenReadModels(adapters)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}
	gormStores, err := newGORMStores(adapters.GlobalView())
	if err != nil {
		return nil, err
	}

	late := &lateStream{}
	ops := operation.New(gormStores.Operations, late.operation, time.Now)
	ax, err := newAggregates(adapters, rm, newParkedReports(ops))
	if err != nil {
		return nil, err
	}
	chatSnapshots := agentusecase.NewChatSnapshots()
	placement, err := newPlacement(ctx, ax, gormStores)
	if err != nil {
		return nil, err
	}
	repos, err := newRepositoriesContainer(ctx, adapters, rm, late, chatSnapshots, ax, placement, engines, purgeGaveUp(ops))
	if err != nil {
		return nil, err
	}
	repoPlacements := dto.NewPlacements(repos.Placement.GetNode, func(ctx context.Context, projectID string, err error) {
		slog.ErrorContext(ctx, "repos: tree unreadable, its repos listed at the root", "project", projectID, "err", err)
	})
	repoRows := &repoEntities{late: late, rows: gormStores.Repositories, placements: repoPlacements}
	gormStores.Projects = &publishingProjects{Store: gormStores.Projects, late: late}
	gormStores.Repositories = publishingRepos{ScopedStore: gormStores.Repositories, entities: repoRows}
	if err := announcePlacement(ax, chatSnapshots, late, repos, repoRows.publish); err != nil {
		return nil, err
	}

	// Path-deriving usecases must share the adapter's resolved home so git
	// worktrees and per-entity storages land under the same root.
	crowbarHome := adapters.Home().Root()
	homeFunc := func() (string, error) { return crowbarHome, nil }
	browser := browserbridge.New()
	ucs, err := usecases.New(
		repos, toUsecaseStores(gormStores), engines, homeFunc, late.reviewThread(),
		usecases.WithChatSnapshots(chatSnapshots),
		usecases.WithBrowserBridge(browser),
	)
	if err != nil {
		return nil, fmt.Errorf("app: usecases: %w", err)
	}
	// The workspace-delete cascade purges each chat through the chat usecase's
	// own PurgeChat, which exists only now (it is built from repos); see
	// repositories.Container.PurgeChat.
	repos.PurgeChat = ucs.AgentChat.PurgeChat
	c := &Container{
		Repositories:   repos,
		GORM:           gormStores,
		Usecases:       ucs,
		Operations:     ops,
		RepoPlacements: repoPlacements,
		Browser:        browser,
		crowbarHome:    crowbarHome,
		engines:        engines,
		axWorkspace:    ax.workspace,
		axReviewThread: ax.reviewThread,
		axAgentChat:    ax.agentChat,
		axTree:         ax.tree,
	}
	c.resources = newStream(ctx, late, encode, ops, repos, ucs, engines, newSidebar(c))
	c.Stream = late.hub
	chatSnapshots.SetPublish(late.chatPublisher(ctx, repos.Workspace, ucs.AgentChat))

	// Before the API opens: nothing a restart interrupted may still read as running.
	if err := c.Operations.Recover(ctx); err != nil {
		return nil, c.abandon(ctx, fmt.Errorf("app: %w", err))
	}
	if err := startBootSweep(ctx, repos, purgeFinished(c.Operations)); err != nil {
		return nil, c.abandon(ctx, err)
	}
	// After Recover, so these runs are not marked interrupted; settled before
	// anything is served (invariant D5), each a recorded operation.
	if err := resumeDeletes(ctx, ucs.ProjectDelete, c.Operations); err != nil {
		return nil, c.abandon(ctx, err)
	}
	startRestoreTerminalSessions(ctx, ucs)
	// Before the owner reconcile (a project home created here needs an owner).
	// After the sweep and resumed deletes, which may purge before the Tree is
	// seeded: placement-tree copies no row of a record already gone.
	impls := postOpenImpls(newLegacyFuncs(repos, ucs, adapters.Home()), placement, legacySources(adapters, repos, gormStores))
	if err := post.RunPostOpen(ctx, impls); err != nil {
		return nil, c.abandon(ctx, fmt.Errorf("app: migrate: %w", err))
	}
	// After the upgrade steps, which can hand it a runner a crashed old daemon left.
	reconcileAgentRunners(ctx, ucs)
	// Loops start once the boot can no longer fail, so a failed one leaves none.
	startProviderSweep(ctx, engines, repos, ucs)
	startOwningChatReconcile(ctx, repos, ucs)
	startTerminalWaitSweep(ctx, late, ucs)
	startModelDiscoveryWarmup(ctx, engines, crowbarHome)
	go descriptorcheck.LogAll(context.WithoutCancel(ctx), crowbarHome)

	return c, nil
}

// newRepositoriesContainer builds the repository layer from every per-type
// asynx singleton and its read model. The fanout built here decides what a
// client is told.
func newRepositoriesContainer(
	ctx context.Context,
	adapters *adapter.Container,
	rm *repositories.ReadModels,
	late *lateStream,
	chatSnapshots *agentusecase.ChatSnapshots,
	ax *aggregates,
	placement *repotree.Ports,
	engines *engine.Container,
	onGiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error),
) (*repositories.Container, error) {
	agentFanout := agentusecase.NewFanout(chatSnapshots)
	repos, err := repositories.New(
		ctx,
		adapters,
		rm,
		late.workspace,
		ax.reviewThread,
		ax.workspace,
		ax.agentChat,
		placement,
		engines.Git,
		agentFanout.ChatWatch(),
		onGiveUp,
	)
	if err != nil {
		return nil, fmt.Errorf("app: repositories: %w", err)
	}
	return repos, nil
}

func toUsecaseStores(
	gormStores *GORMStores,
) usecases.GORMStores {
	return usecases.GORMStores{
		Projects:                 gormStores.Projects,
		Repositories:             gormStores.Repositories,
		TerminalProfiles:         gormStores.TerminalProfiles,
		TerminalSessions:         gormStores.TerminalSessions,
		AgentProviderPreferences: gormStores.AgentProviderPreferences,
		AgentPermissionDefault:   gormStores.AgentPermissionDefault,
		AgentModelManifestFetch:  gormStores.AgentModelManifestFetch,
		AgentChatTelemetry:       gormStores.AgentChatTelemetry,
	}
}

// startTerminalWaitSweep starts the cadence that notices a vendor CLI parked on a modal
// (e.g. workspace trust) that sends no hook. It lives here, not in a usecase, because it
// publishes through the stream, and runs after reconcileAgentRunners to skip dead rows.
func startTerminalWaitSweep(
	ctx context.Context,
	late *lateStream,
	ucs *usecases.Container,
) {
	ucs.AgentRunner.StartTerminalWaitSweep(ctx, agentusecase.ChatFeed{
		Telemetry: late.telemetry,
	})
}

// startModelDiscoveryWarmup kicks each descriptor's live model probe at boot instead
// of on the first providers request, off the boot path. The List call is detached from
// ctx so an early cancel cannot abort it, but its forked refreshes still stop at shutdown.
func startModelDiscoveryWarmup(
	ctx context.Context,
	engines *engine.Container,
	crowbarHome string,
) {
	if engines == nil || engines.Agents == nil {
		return
	}
	go func() {
		if _, err := engines.Agents.List(context.WithoutCancel(ctx), crowbarHome); err != nil {
			slog.WarnContext(ctx, "app: model discovery warmup", "err", err)
		}
	}()
}

func startProviderSweep(
	ctx context.Context,
	engines *engine.Container,
	repos *repositories.Container,
	ucs *usecases.Container,
) {
	engines.Provider.StartBackgroundSweep(
		ctx,
		sweepTargets(repos.Workspace),
		sweepCallback(ctx, ucs),
	)
}

// startBootSweep synchronously reaps, before serving, every workspace left tombstoned
// by a delete whose reactor never finished, re-driving the same Purger. A missing purger
// is a wiring error and surfaces; per-row failures are logged and never fail boot.
func startBootSweep(
	ctx context.Context,
	repos *repositories.Container,
	onPurged func(ctx context.Context, wsID string),
) error {
	sweeper, ok := repos.Workspace.(workspace.BootSweeper)
	if !ok {
		return nil
	}
	sweeper.BackfillProvisioning(ctx)
	if err := sweeper.Sweep(ctx, onPurged); err != nil {
		return fmt.Errorf("app: boot sweep: %w", err)
	}
	return nil
}

// startRestoreTerminalSessions reloads persisted terminal sessions as PTY-less
// placeholders a later Attach restores. It runs synchronously before serving so no
// Attach races the restore; per-row errors are logged.
func startRestoreTerminalSessions(
	ctx context.Context,
	ucs *usecases.Container,
) {
	if ucs.Terminal == nil {
		return
	}
	if err := ucs.Terminal.RestorePersistedSessions(context.WithoutCancel(ctx)); err != nil {
		slog.WarnContext(ctx, "app: restore persisted terminal sessions", "err", err)
	}
}

// reconcileAgentRunners ends the runners the ledger still holds open from the previous
// run: their PTYs died with it, and an open one keeps its chat busy. It runs before
// serving; a failure is logged rather than blocking boot, which would be strictly worse.
func reconcileAgentRunners(
	ctx context.Context,
	ucs *usecases.Container,
) {
	if ucs.AgentRunner == nil {
		return
	}
	if err := ucs.AgentRunner.ReconcileRunnersOnBoot(context.WithoutCancel(ctx)); err != nil {
		slog.WarnContext(ctx, "app: reconcile agent runners on boot", "err", err)
	}
}

// startOwningChatReconcile runs the D4 boot reconcile (owning_chats_reconcile.go)
// SYNCHRONOUSLY, before anything is served. A listing failure is logged: the
// daemon still boots, and the next boot tries again.
func startOwningChatReconcile(
	ctx context.Context,
	repos *repositories.Container,
	ucs *usecases.Container,
) {
	if ucs.AgentChat == nil || ucs.AgentChatFolder == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	workspaces, err := repos.Workspace.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "app: boot: list workspaces for the owning-chat reconcile", "err", err)
		return
	}
	chats, err := ucs.AgentChat.ListChats(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "app: boot: list chats for the owning-chat reconcile", "err", err)
		return
	}
	reconcileOwningChats(ctx, workspaces, chats, ucs.AgentChatFolder)
}
