package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/realtime"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// Container is the application layer: the hub, the aggregate repositories, the
// GORM CRUD stores, the composed usecases, and the realtime service owning the
// lazy file-watcher and LSP lifecycles.
type Container struct {
	Hub          *hub.Hub
	Repositories *repositories.Container
	GORM         *GORMStores
	Usecases     *usecases.Container
	Realtime     *realtime.Service

	// axWorkspace, axReviewThread, and axAgentChat are the per-type asynx
	// singletons (one per aggregate type, routing every id by shard hash). They
	// are retained here so Task 15's ordered graceful shutdown can drain each via
	// ax.Shutdown. axAgentChat is additive (Task 9): its store/hub projections
	// are live, but the usecase does not consume it yet — that's a later cutover.
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.AgentChat]
}

// New constructs the application layer from the engine and adapter containers
// and wires the aggregate repositories into the hub (00 §7).
func New(
	ctx context.Context,
	engines *engine.Container,
	adapters *adapter.Container,
) (*Container, error) {
	axReviewThread, err := newAsynx[domain.ReviewThread](adapters.ReviewThreadES())
	if err != nil {
		return nil, fmt.Errorf("app: asynx review thread: %w", err)
	}
	// One eager axWorkspace singleton over the per-type event log, routing every
	// workspace id to a shard by hash (decision 1) — replaces the per-entity
	// AsynxFactory the repository used to resolve per workspace.
	axWorkspace, err := newAsynx[domain.Workspace](adapters.WorkspaceES())
	if err != nil {
		return nil, fmt.Errorf("app: asynx workspace: %w", err)
	}
	// axAgentChat is the per-type singleton over state/events/agent_chat.db: it
	// is built and its store/hub projections registered (via repositories.New ->
	// agentchat.NewEventSourced), and the agent usecase now sends every AgentChat
	// mutation through it (the gorm-backed store was retired in the Task 10
	// cutover).
	axAgentChat, err := newAsynx[domain.AgentChat](adapters.AgentChatES())
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent chat: %w", err)
	}

	gormStores, err := newGORMStores(adapters.GlobalView())
	if err != nil {
		return nil, err
	}

	h := hub.NewHub()
	repos, err := repositories.New(
		ctx,
		adapters,
		h,
		axReviewThread,
		axWorkspace,
		axAgentChat,
		engines.Git,
		terminateAgentSession(engines.Terminal),
	)
	if err != nil {
		return nil, fmt.Errorf("app: repositories: %w", err)
	}

	// Path-deriving usecases must share the adapter's resolved home so git
	// worktrees and per-entity storages land under the same root.
	crowbarHome := adapters.CrowbarHome()
	homeFunc := func() (string, error) { return crowbarHome, nil }
	ucs, err := usecases.New(repos, toUsecaseStores(gormStores), engines, homeFunc)
	if err != nil {
		return nil, fmt.Errorf("app: usecases: %w", err)
	}

	startProviderSweep(ctx, engines, repos, ucs)
	if err := startBootSweep(ctx, adapters, repos, axWorkspace); err != nil {
		return nil, err
	}
	startRestoreTerminalSessions(ctx, ucs)
	seedAgentRegistry(ctx, ucs)
	reconcileAgentBoot(ctx, ucs)
	sweepStaleAgentTmp(crowbarHome)

	rt := realtime.New(
		ctx,
		h,
		repos.Workspace,
		engines.Git,
		engines.FS,
		realtime.NewLSPLifecycle(engines.LSP),
		ucs.ProviderSync,
		poll.PerConnectionInterval,
		realtime.OriginSyncInterval,
		time.Now,
	)

	return &Container{
		Hub:            h,
		Repositories:   repos,
		GORM:           gormStores,
		Usecases:       ucs,
		Realtime:       rt,
		axWorkspace:    axWorkspace,
		axReviewThread: axReviewThread,
		axAgentChat:    axAgentChat,
	}, nil
}

// Shutdown gracefully quiesces the application layer's asynchronous machinery
// within the ctx deadline, BEFORE the adapter closes the DBs (spec §3.8 steps
// 2-3, decision 11). It runs ahead of Close in the ordered teardown:
//
//  1. close the shared reactor drain gate (Cancel) so no post-commit reactor
//     wireCallbacks registered starts new work;
//  2. wait out the in-flight reactors, BOUNDED by ctx — a hung reactor cannot
//     wedge shutdown past the deadline (quiver's drainWg.Wait() is unbounded; we
//     bound it, decision 11);
//  3. Shutdown each per-type asynx singleton, draining its command/projection
//     pool (itself ctx-bounded) so no event is half-processed when the adapter
//     WAL-checkpoints and closes the event/read/view DBs.
//
// Realtime resources (file watchers, LSP hosts) are released separately by Close.
// Every wait honors ctx, so the whole drain is bounded by the caller's deadline.
func (c *Container) Shutdown(
	ctx context.Context,
) error {
	drain := c.Repositories.Drain()
	drain.Cancel() // close the gate: reactors observing drainCtx stop starting new work.

	done := make(chan struct{})
	go func() {
		drain.WG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// Bounded: a stuck reactor cannot hang shutdown past the deadline.
	}

	return errors.Join(
		c.axWorkspace.Shutdown(ctx),
		c.axReviewThread.Shutdown(ctx),
		c.axAgentChat.Shutdown(ctx),
	)
}

// Close tears down the application layer's live realtime resources: it stops
// every file watcher and LSP host the service still holds. It is idempotent and
// runs on graceful shutdown so fsnotify file descriptors and LSP subprocesses
// are released promptly.
func (c *Container) Close() {
	c.Realtime.Close()
}

// terminateAgentSession adapts the terminal engine's TerminateGraceful into the
// plain func(ctx, sessionID) error seam the workspace-delete cascade
// (repositories.Container.forgetAgentChats) uses to kill a chat's live PTY
// before Forgetting it (Task 12). Threading the raw engine straight into
// repositories.New mirrors how engines.Git is already threaded in (as
// wsusecase.MergeConflictChecker) BEFORE the usecases layer — which also
// builds the agent usecase's own TerminalCommander — exists: the repositories
// package itself gains no terminal-engine dependency, only this narrow func.
// A session already gone (engineterminal.ErrSessionNotFound — e.g. the CLI
// exited on its own) is not a real failure and is swallowed here, matching
// how the agent usecase's own SwitchProvider treats the exact same error.
func terminateAgentSession(
	term engineterminal.Engine,
) func(ctx context.Context, sessionID string) error {
	return func(ctx context.Context, sessionID string) error {
		if err := term.TerminateGraceful(ctx, sessionID); err != nil &&
			!errors.Is(err, engineterminal.ErrSessionNotFound) {
			return err
		}
		return nil
	}
}

func toUsecaseStores(
	gormStores *GORMStores,
) usecases.GORMStores {
	return usecases.GORMStores{
		Projects:         gormStores.Projects,
		Repositories:     gormStores.Repositories,
		TerminalProfiles: gormStores.TerminalProfiles,
		TerminalSessions: gormStores.TerminalSessions,
	}
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

// startBootSweep runs the cheap, proactive boot orphan-sweep (spec §3.8)
// SYNCHRONOUSLY before app.New returns (and thus before internal.Run serves) —
// replacing the old async recovery sweep. It reaps any workspace stuck in
// Status="deleted" by a delete reactor that crashed mid-cascade: it reads
// store/workspace.db DIRECTLY (no lazy Replay, §3.7) and re-drives the idempotent
// purge (rm -rf worktree + drop the id↔path row + axWorkspace.Forget), converging
// to the delete invariant (no row, no worktree) whichever teardown step the crash
// interrupted. The review-thread forget cascade is registered separately by
// wireCallbacks (Task 14); the primary crash gap this closes is the lingering
// worktree/row. Best-effort: recovery work never fails boot, but a failure to
// construct the id↔path store is a real wiring error and is surfaced.
func startBootSweep(
	ctx context.Context,
	adapters *adapter.Container,
	repos *repositories.Container,
	ax asynx.Asynx[domain.Workspace],
) error {
	sweeper, ok := repos.Workspace.(workspace.BootSweeper)
	if !ok {
		return nil
	}
	pathsStore, err := wspaths.NewWorkspacePaths(adapters.GlobalView())
	if err != nil {
		return fmt.Errorf("app: boot sweep: paths store: %w", err)
	}
	sweeper.Sweep(ctx, bootSweepPurge(ax, pathsStore, adapters.CrowbarHome()))
	return nil
}

// bootSweepPurge builds the idempotent teardown the boot orphan-sweep re-drives
// for each residual Status="deleted" workspace (spec §3.8): resolve the worktree
// path from the id↔path map, rm -rf it, delete the id↔path row (§3.9 write-point
// c), then Forget the aggregate — whose synchronous OnForget drops the read-model
// row — as the terminal step. Every step is idempotent so a re-drive after a
// crash is a no-op: a missing path row skips the rm, an absent worktree rm's to
// nothing, and Forgetting an already-Forgotten aggregate (ErrValidation) is
// swallowed. It mirrors the delete reactor's purge (§3.6) minus the tombstone
// gate — the sweep already selected rows the projection persisted as "deleted".
func bootSweepPurge(
	ax asynx.Asynx[domain.Workspace],
	pathsStore wspaths.WorkspacePaths,
	crowbarHome string,
) func(ctx context.Context, wsID string) error {
	return func(ctx context.Context, wsID string) error {
		path, err := pathsStore.Get(ctx, wsID)
		switch {
		case errors.Is(err, wspaths.ErrNotFound):
			path = ""
		case err != nil:
			return fmt.Errorf("resolve worktree path: %w", err)
		}
		// GUARD: only rm a crowbar-managed worktree (strictly under the home). An
		// adopted home/main worktree's mapped path is the user's REAL checkout
		// (repo.Path/project.Path, outside the home) and must never be destroyed —
		// so a crash-recovery re-drive of a tombstoned home never deletes a user's
		// repository (mirrors the delete reactor's worktreeRemover guard).
		if path != "" && crowbarHome != "" && strings.HasPrefix(path, strings.TrimRight(crowbarHome, "/")+"/") {
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("rm worktree %q: %w", path, err)
			}
		}
		if err := pathsStore.Delete(ctx, wsID); err != nil {
			return fmt.Errorf("delete id-path row: %w", err)
		}
		if err := ax.Forget(ctx, wsID); err != nil && !errors.Is(err, asynxModels.ErrValidation) {
			return fmt.Errorf("forget aggregate: %w", err)
		}
		return nil
	}
}

// startRestoreTerminalSessions reloads persisted terminal sessions as PTY-less
// placeholders so a subsequent client Attach transparently restores them.
// FIX 3: runs SYNCHRONOUSLY before the engine/HTTP layer starts serving, so
// the registry is fully populated before the first Attach can arrive. Running
// it in the background allowed concurrent Attach + restore races. Best-effort:
// per-row errors are logged; orphaned rows are reconciled automatically.
func startRestoreTerminalSessions(
	ctx context.Context,
	ucs *usecases.Container,
) {
	if ucs.Terminal == nil {
		return
	}
	_ = ucs.Terminal.RestorePersistedSessions(context.WithoutCancel(ctx))
}

// seedAgentRegistry rehydrates the agent usecase's context-move reducer from
// persisted segments (see agent.Usecase.SeedRegistry's doc comment): it runs
// synchronously at startup, before the HTTP layer starts serving, so a
// resumed vendor-CLI process that /resumes into a pre-restart chat is
// recognized as a "focus" move rather than mistakenly "registered" as new.
// Best-effort: a failure here only degrades context-move detection for
// already-running segments, it never blocks startup.
func seedAgentRegistry(
	ctx context.Context,
	ucs *usecases.Container,
) {
	if ucs.Agent == nil {
		return
	}
	_ = ucs.Agent.SeedRegistry(context.WithoutCancel(ctx))
}

// reconcileAgentBoot repairs live turn state a daemon crash can leave stale
// (see agent.Usecase.ReconcileOnBoot's doc comment): no event records "the
// CLI process died," so a chat's active segment / Working flag can survive a
// restart pointing at a terminal session that no longer exists. It runs
// synchronously at startup, AFTER startRestoreTerminalSessions has
// repopulated the terminal registry — so a session merely reloaded as a
// suspended placeholder still reads alive via SessionExists and is never
// wrongly reconciled — and before the HTTP layer starts serving. Best-effort:
// a failure here only leaves a chat's live-turn state stale until the next
// hook/switch touches it, it never blocks startup.
func reconcileAgentBoot(
	ctx context.Context,
	ucs *usecases.Container,
) {
	if ucs.Agent == nil {
		return
	}
	if err := ucs.Agent.ReconcileOnBoot(context.WithoutCancel(ctx)); err != nil {
		slog.WarnContext(ctx, "app: reconcile agent boot", "err", err)
	}
}

// sweepStaleAgentTmp best-effort removes <home>/agent-tmp at daemon startup.
// agent.Usecase.spawnSegment renders each spawned segment's hook config (and,
// for codex, a COPY of ~/.codex/auth.json — a credential) into
// <home>/agent-tmp/<segID>, kept alive for the whole life of the running
// vendor CLI and removed via the terminal engine's onExit callback when that
// CLI's PTY session ends. No agentic PTY survives a daemon restart, so on the
// next startup every entry under agent-tmp is guaranteed stale — sweeping the
// whole directory (rather than resolving the home from persisted segments,
// which SeedRegistry has no clean path to) is both simpler and sufficient. It
// runs synchronously, before the HTTP layer starts serving, so it can never
// race a freshly spawned segment's own tmp dir. Best-effort: an error here
// only leaves a slightly larger stale-dir backlog for the following restart,
// never blocks startup.
func sweepStaleAgentTmp(crowbarHome string) {
	_ = os.RemoveAll(filepath.Join(crowbarHome, "agent-tmp"))
}

func sweepCallback(
	ctx context.Context,
	ucs *usecases.Container,
) func(
	wsID string,
	state provider.ProviderState,
) {
	return func(
		wsID string,
		state provider.ProviderState,
	) {
		_ = ucs.ProviderSync.SyncFromState(
			context.WithoutCancel(ctx),
			wsID,
			state,
			time.Now(),
		)
	}
}

// shouldSweep reports whether the global cron should re-poll a workspace: it
// must have a live PR (PRUrl != "") and must not be in a terminal PR state
// (pr-merged/pr-closed), which are never re-polled (D10/§11). This widens the
// old Status==pr-open filter so pr-open->pr-merged/closed transitions are
// observed on unwatched workspaces and pr-conflicts workspaces keep syncing.
func shouldSweep(
	ws domain.Workspace,
) bool {
	if ws.PRUrl == "" {
		return false
	}
	if ws.Status == domain.WorkspaceStatusPRMerged ||
		ws.Status == domain.WorkspaceStatusPRClosed {
		return false
	}
	return true
}

func sweepTargets(
	repo workspace.Workspace,
) func() []poll.SweepTarget {
	return func() []poll.SweepTarget {
		rows, err := repo.List(context.Background())
		if err != nil {
			return nil
		}
		targets := make([]poll.SweepTarget, 0, len(rows))
		for _, ws := range rows {
			if !shouldSweep(ws) {
				continue
			}
			targets = append(targets, poll.SweepTarget{
				WSID:      ws.ID,
				RepoPath:  ws.WorktreePath,
				Branch:    ws.Branch,
				HasOpenPR: true,
			})
		}
		return targets
	}
}
