package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/realtime"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
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

	// engines is retained for ONE reason: Shutdown must quiesce the terminal engine
	// (kill every PTY and JOIN the exit callbacks those deaths fire) BEFORE it drains
	// the aggregates below, because those callbacks are writers into them. See
	// Shutdown step 1.
	engines *engine.Container

	// axWorkspace, axReviewThread, axAgentChat, and axAgentRunner are the per-type
	// asynx singletons (one per aggregate type, routing every id by shard hash).
	// They are retained here so Task 15's ordered graceful shutdown can drain each
	// via ax.Shutdown. axAgentRunner carries the RUNNER aggregate — one live vendor CLI
	// in one PTY, the thing that moves between chats on /clear and /resume.
	axWorkspace    asynx.Asynx[domain.Workspace]
	axReviewThread asynx.Asynx[domain.ReviewThread]
	axAgentChat    asynx.Asynx[domain.AgentChat]
	// axAgentActivity is the conversation record's own per-type singleton. It is
	// separate from axAgentChat because their write rates differ by orders of
	// magnitude: a chat emits a handful of events, its activity emits hundreds per
	// turn, and sharing one single-writer event log would put a sidebar repaint
	// behind a tool-call storm.
	axAgentActivity asynx.Asynx[domain.AgentActivity]
	axAgentRunner   asynx.Asynx[agents.Runner]
}

// New constructs the application layer from the engine and adapter containers
// and wires the aggregate repositories into the hub (00 §7).
func New(
	ctx context.Context,
	engines *engine.Container,
	adapters *adapter.Container,
) (*Container, error) {
	axReviewThread, err := newAsynx[domain.ReviewThread](adapters.ReviewThreadES(), adapters.ReviewThreadSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx review thread: %w", err)
	}
	// One eager axWorkspace singleton over the per-type event log, routing every
	// workspace id to a shard by hash (decision 1) — replaces the per-entity
	// AsynxFactory the repository used to resolve per workspace.
	axWorkspace, err := newAsynx[domain.Workspace](adapters.WorkspaceES(), adapters.WorkspaceSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx workspace: %w", err)
	}
	// axAgentChat is the per-type singleton over state/events/agent_chat.db: it
	// is built and its store/hub projections registered (via repositories.New ->
	// agentchat.NewEventSourced), and the agent usecase now sends every AgentChat
	// mutation through it (the gorm-backed store was retired in the Task 10
	// cutover).
	axAgentChat, err := newAsynx[domain.AgentChat](adapters.AgentChatES(), adapters.AgentChatSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent chat: %w", err)
	}
	// axAgentRunner is the per-type singleton over state/events/agent_runner.db: the
	// running vendor CLI is now a modelled aggregate of its own, so moving one
	// between chats (/clear, /resume) is a single write to a single aggregate
	// instead of a delete-here/insert-there across two chats with no transaction.
	// Built and its projections registered (via repositories.New ->
	// runner.NewEventSourced); nothing SENDS runner commands yet — that
	// cutover is a later task — so it is additive for now.
	axAgentActivity, err := newAsynx[domain.AgentActivity](
		adapters.AgentActivityES(), adapters.AgentActivitySS(),
	)
	if err != nil {
		return nil, fmt.Errorf("app container: agent activity asynx: %w", err)
	}

	axAgentRunner, err := newAsynx[agents.Runner](adapters.AgentRunnerES(), adapters.AgentRunnerSS())
	if err != nil {
		return nil, fmt.Errorf("app: asynx agent runner: %w", err)
	}

	gormStores, err := newGORMStores(adapters.GlobalView())
	if err != nil {
		return nil, err
	}

	h := hub.NewHub()
	// The agent aggregates announce; the fanout decides what a client is told. The hub
	// still reaches the repository layer for workspace frames, which are outside this
	// subsystem.
	agentFanout := agent.NewFanout(h)
	repos, err := repositories.New(
		ctx,
		adapters,
		h,
		axReviewThread,
		axWorkspace,
		axAgentChat,
		axAgentActivity,
		axAgentRunner,
		engines.Git,
		terminateAgentSession(engines.Terminal),
		agentFanout.ChatWatch(),
		agentFanout.RunnerWatch(),
	)
	if err != nil {
		return nil, fmt.Errorf("app: repositories: %w", err)
	}

	// Path-deriving usecases must share the adapter's resolved home so git
	// worktrees and per-entity storages land under the same root.
	crowbarHome := adapters.CrowbarHome()
	homeFunc := func() (string, error) { return crowbarHome, nil }
	ucs, err := usecases.New(repos, toUsecaseStores(gormStores), engines, homeFunc, agentThreadBroadcast(h))
	if err != nil {
		return nil, fmt.Errorf("app: usecases: %w", err)
	}
	// Wire the workspace-delete cascade's on-disk reap seam now that the agent
	// usecase's WorkspaceReader exists (repos.Workspace, which it resolves
	// against, is only populated once repositories.New above has returned — see
	// repositories.Container.ReapChatFiles' doc comment for why this can't be a
	// repositories.New constructor argument).
	repos.ReapChatFiles = reapAgentChatFiles(ucs.AgentWorkspaceReader)

	startProviderSweep(ctx, engines, repos, ucs)
	if err := startBootSweep(ctx, adapters, repos, axWorkspace); err != nil {
		return nil, err
	}
	startRestoreTerminalSessions(ctx, ucs)
	reconcileAgentRunners(ctx, ucs)
	startTerminalWaitSweep(ctx, h, ucs)

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
		Hub:             h,
		Repositories:    repos,
		GORM:            gormStores,
		Usecases:        ucs,
		Realtime:        rt,
		engines:         engines,
		axWorkspace:     axWorkspace,
		axReviewThread:  axReviewThread,
		axAgentChat:     axAgentChat,
		axAgentActivity: axAgentActivity,
		axAgentRunner:   axAgentRunner,
	}, nil
}

// Shutdown gracefully quiesces the application layer's asynchronous machinery
// within the ctx deadline, BEFORE the adapter closes the DBs (spec §3.8 steps
// 2-3, decision 11). It runs ahead of Close in the ordered teardown, and its whole
// job is to leave every WRITER quiesced before the things they write to go away —
// in dependency order, outermost writer first:
//
//  1. quiesce the terminal engine: kill every live PTY and JOIN the exit callbacks
//     those deaths fire. This is FIRST, and it is not a resource release (Close
//     does that) — it is the outermost writer. A dying vendor CLI's exit callback
//     is the ONLY thing that records its death: it Exits the runner and closes the
//     turn the CLI abandoned. It runs on the terminal engine's reap goroutine, so
//     leaving it to Close — after steps 2-3 and the adapter's DB close — is a
//     RACE, and one that was lost in production ("close abandoned turn: get chat …
//     sql: database is closed"). Losing it is worse than a lost warning: the two
//     writes fall on opposite sides of the teardown, so the runner's Exit commits
//     while its turn is NOT closed — and with no live runner row left, the next
//     boot's ReconcileRunnersOnBoot has nothing to reconcile, so NOTHING ever
//     closes that turn. The chat spins forever, across every restart, and takes
//     the workspace's working overlay with it;
//  2. close the shared reactor drain gate (Cancel) so no post-commit reactor
//     wireCallbacks registered starts new work;
//  3. wait out the in-flight reactors, BOUNDED by ctx — a hung reactor cannot
//     wedge shutdown past the deadline (quiver's drainWg.Wait() is unbounded; we
//     bound it, decision 11);
//  4. Shutdown each per-type asynx singleton, draining its command/projection
//     pool (itself ctx-bounded) so no event is half-processed when the adapter
//     WAL-checkpoints and closes the event/read/view DBs. Step 1's writes are
//     folded into the read models here, which is the other half of why it must
//     precede this: an Exit sent AFTER an asynx Shutdown is simply rejected
//     ("asynx: shutting down") and the death goes unrecorded.
//
// Note what step 1 is NOT: it is not a second opinion about liveness. The PTY
// remains the sole authority — we kill the process and let its death carry the
// runner away, exactly as every other teardown path does. All we have changed is
// that the daemon now WAITS to hear the news before it dismantles the ears.
//
// Realtime resources (file watchers, LSP hosts) are released separately by Close.
// Every wait honors ctx, so the whole drain is bounded by the caller's deadline.
func (c *Container) Shutdown(
	ctx context.Context,
) error {
	// FIRST, before anything below can be cut short by ctx: this is the only
	// moment the agent tool counters are ever read.
	c.logAgentToolUsage()

	c.quiesceTerminal(ctx)

	drain := c.Repositories.Drain()
	drain.Cancel() // close the gate: reactors observing drainCtx stop starting new work.

	// Bounded by ctx: a stuck reactor delays shutdown by the deadline and no longer.
	//
	// The gate — not a bare WaitGroup — is what makes this safe. Step 1 above kills the
	// PTYs, and those deaths COMMIT EVENTS, which wake reactors: the daemon is at its most
	// eventful in the very instant it is trying to go quiet. A reactor Adding on asynx's
	// bus goroutine while this line Waits is textbook WaitGroup misuse, and `-race` caught
	// it. drain.Gate makes admitting work and beginning to drain one critical section.
	drain.Gate.Wait(ctx)

	// Order matters on the ABANDON path (the clean path already drained every reaper in
	// quiesceTerminal above, so all its writes have landed). If a reaper is still in flight
	// when we get here — quiesceTerminal was cut short by ctx — it writes runners.Exit
	// FIRST and chats.StopTurn SECOND (reconcileRunnerExit), and it RETURNS EARLY if the
	// first write is rejected. So shut the RUNNER store down before the CHAT store: a
	// reaper that loses the race then has its Exit rejected and bails before StopTurn,
	// leaving the runner row LIVE for the next boot's reconcile to retry both writes. The
	// reverse order lets Exit commit while StopTurn is rejected — runner gone, turn
	// stranded, and boot reconcile has no live row left to find. Recoverable vs permanent.
	return errors.Join(
		c.axWorkspace.Shutdown(ctx),
		c.axReviewThread.Shutdown(ctx),
		c.axAgentRunner.Shutdown(ctx),
		c.axAgentChat.Shutdown(ctx),
		c.axAgentActivity.Shutdown(ctx),
	)
}

// logAgentToolUsage emits the one and only read of the agent capability
// surface's call counters, as a single line at shutdown:
//
//	agent tool usage this boot  tools="post_review_comment=7/1 set_chat_title=3/0"
//
// (tool=calls/failures, sorted, so consecutive boots diff cleanly.)
//
// The counters exist to settle whether agents actually USE these tools — the
// shell command this surface replaces is known to be ignored by real models —
// and a counter nothing reads settles nothing. Shutdown is the right and only
// place: the numbers are cumulative over a daemon's lifetime, so this is the
// moment they are complete. A boot that saw no tool call logs nothing rather
// than an empty line.
//
// Deliberately not an HTTP route. This is a diagnostic about the daemon, not a
// resource of the product, and nothing in the UI consumes it.
func (c *Container) logAgentToolUsage() {
	if c.Usecases == nil {
		return
	}
	stats := c.Usecases.AgentToolMetrics()
	if len(stats) == 0 {
		return
	}
	names := make([]string, 0, len(stats))
	for name := range stats {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d/%d", name, stats[name].Calls, stats[name].Failures))
	}
	slog.Info("agent tool usage this boot (tool=calls/failures)", "tools", strings.Join(parts, " "))
}

// quiesceTerminal runs Shutdown's step 1 — kill the PTYs, join their exit callbacks
// — BOUNDED by ctx, mirroring how the reactor drain below it is bounded (decision
// 11): a reaper wedged on something we do not control must not hold the daemon past
// its deadline. The quiesce is latched in the engine container, so the later
// engines.Close() never re-enters (and so never re-wedges) whatever we abandon here.
func (c *Container) quiesceTerminal(
	ctx context.Context,
) {
	if c.engines == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.engines.QuiesceTerminal()
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
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

// reapAgentChatFiles adapts the agent usecase's WorkspaceReader (AgentChatsDir +
// WorktreeDir) into the plain func(ctx, wsID, chatID) error seam
// repositories.Container.ReapChatFiles wants: the workspace-delete cascade
// (repositories.Container.forgetAgentChats) calls it, per forgotten chat, to
// remove that chat's own <chatsDir>/<chatID> directory. It reuses the EXACT
// same path resolution and agent.RemoveUnderHome guard the standalone
// PurgeChat path already routes through (Important-2) — no path logic is
// reimplemented here — so a workspace delete no longer leaves a chat's
// plaintext handoff ledger behind under .crowbar. ws is the SAME reader
// instance the agent usecase itself was built with (see
// usecases.Container.AgentWorkspaceReader's doc comment for why this is wired
// in after usecases.New returns rather than threaded into repositories.New).
func reapAgentChatFiles(
	ws agent.WorkspaceReader,
) func(ctx context.Context, wsID, chatID string) error {
	return func(ctx context.Context, wsID, chatID string) error {
		chatsDir, err := ws.AgentChatsDir(ctx, wsID)
		if err != nil {
			return fmt.Errorf("resolve chats dir: %w", err)
		}
		home, _, _, _, err := ws.WorktreeDir(ctx, wsID)
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
		agent.RemoveUnderHome(ctx, home, filepath.Join(chatsDir, chatID))
		return nil
	}
}

// agentThreadBroadcast adapts the hub into the agenttools.ThreadBroadcast seam:
// when an agent posts a review comment, the resulting thread has to reach a review
// pane that is already open, exactly as an HTTP-authored comment does.
//
// The conversion lives HERE, in the app layer, because it is the DTO boundary. The
// review-thread repository does not fan out (its store.BroadcastFunc is a no-op)
// and it cannot: the frame is built from domain.ReviewThread, which carries WsID but
// no project or repo id, while the /threads stream filters on all three. Only a
// caller holding the resolved workspace can supply them, so the aggregate crosses
// the usecase boundary and the DTO is assembled at the layer that owns wire types.
//
// This does NOT double-broadcast alongside the thread handler's own push: both end
// at the same ws.Broadcaster, but the agent path never runs the handler, and the
// handler's path never runs this.
func agentThreadBroadcast(
	h threadBroadcaster,
) agenttools.ThreadBroadcast {
	return func(thread domain.ReviewThread, projectID, repoID string) {
		h.BroadcastThread(dto.ThreadDTOFrom(thread, projectID, repoID))
	}
}

// threadBroadcaster is the one hub method agentThreadBroadcast needs, narrowed to
// it so nothing else about the hub is in scope here. *hub.Hub satisfies it.
type threadBroadcaster interface {
	BroadcastThread(
		t dto.ThreadDTO,
	)
}

func toUsecaseStores(
	gormStores *GORMStores,
) usecases.GORMStores {
	return usecases.GORMStores{
		Projects:                 gormStores.Projects,
		Repositories:             gormStores.Repositories,
		Folders:                  gormStores.Folders,
		AgentChatFolders:         gormStores.AgentChatFolders,
		TerminalProfiles:         gormStores.TerminalProfiles,
		TerminalSessions:         gormStores.TerminalSessions,
		AgentProviderPreferences: gormStores.AgentProviderPreferences,
	}
}

// startTerminalWaitSweep begins the cadence that notices a vendor CLI parked on a
// modal Crowbar cannot answer — the workspace-trust dialog and its relatives, which
// reach the daemon through no hook and otherwise leave a chat pane showing nothing
// at all over a live, blocked process.
//
// Started HERE, beside the boot reconcile, rather than inside the usecase: the
// detector publishes, and the thing it publishes through is the hub, which is a
// layer above every usecase. It is also why the sweep begins after
// reconcileAgentRunners — a restart's first census should be of runners already
// reconciled, not of rows the reconcile is about to retire.
func startTerminalWaitSweep(
	ctx context.Context,
	h *hub.Hub,
	ucs *usecases.Container,
) {
	ucs.AgentRunner.StartTerminalWaitSweep(
		ctx,
		func(chatID, workspaceID string, wait domain.AgentTerminalWait) {
			h.BroadcastAgentChatTerminalWait(chatID, workspaceID, dto.TerminalWaitDTOFrom(wait))
		},
		func(chatID, workspaceID, requestID string) {
			h.BroadcastAgentChatPromptSettled(chatID, workspaceID, requestID)
		},
		func(chatID, workspaceID, messageID, text string) {
			h.BroadcastAgentChatMessageDelta(chatID, workspaceID, messageID, text)
		},
	)
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
	sweeper.Sweep(ctx, bootSweepPurge(ax, pathsStore, adapters.CrowbarHome(), repos.ForgetWorkspaceDependents))
	return nil
}

// bootSweepPurge builds the idempotent teardown the boot orphan-sweep re-drives
// for each residual Status="deleted" workspace (spec §3.8): resolve the worktree
// path from the id↔path map, rm -rf its workspace root, delete the id↔path row
// (§3.9 write-point c), then Forget the aggregate — whose synchronous OnForget
// drops the read-model row — as the terminal step. Every step is idempotent so a
// re-drive after a crash is a no-op: a missing path row skips the rm, an absent
// worktree rm's to nothing, and Forgetting an already-Forgotten aggregate
// (ErrValidation) is swallowed. It mirrors the delete reactor's purge (§3.6)
// minus the tombstone gate — the sweep already selected rows the projection
// persisted as "deleted" — including the workspace-root parent-dir rm (see
// repositories.worktreeRemover's doc comment: path is the "worktree" leaf of a
// root that also holds the sibling "chats" tree, so the PARENT is what's
// removed, computed inline via filepath.Dir since the internal worktreepath
// package cannot be imported from this package either).
func bootSweepPurge(
	ax asynx.Asynx[domain.Workspace],
	pathsStore wspaths.WorkspacePaths,
	crowbarHome string,
	forgetDependents func(ctx context.Context, wsID string) error,
) func(ctx context.Context, wsID string) error {
	return func(ctx context.Context, wsID string) error {
		// Re-drive the dependent forget-cascade FIRST, while the chats are still listable
		// (their read rows survive the workspace rm below). A tombstone reaches this sweep
		// precisely because its reactor did not finish — crashed mid-cascade, or was refused
		// by the drain gate at shutdown — so its chat aggregates and conversation rows are
		// exactly what is still un-forgotten. Abort on error (leave the tombstone for the
		// next re-drive) rather than rm the worktree with chats still dangling; that is the
		// same contract forgetDependents holds on the reactor path.
		if err := forgetDependents(ctx, wsID); err != nil {
			return fmt.Errorf("forget workspace dependents: %w", err)
		}
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
		// repository (mirrors the delete reactor's worktreeRemover guard). The rm
		// target is the PARENT (workspace root holding the sibling chats tree), so
		// the SAME strict-under-home test is applied to the root too: a degenerate
		// one-segment leaf (<home>/worktree) has filepath.Dir == home, and rm'ing
		// that would nuke the entire crowbar home — refused here.
		if underHome(path, crowbarHome) {
			root := filepath.Dir(path)
			if !underHome(root, crowbarHome) {
				slog.Warn("app: boot sweep: refusing to rm workspace root at or above the crowbar home",
					"root", root, "path", path, "home", crowbarHome)
			} else if err := os.RemoveAll(root); err != nil {
				return fmt.Errorf("rm workspace root %q: %w", root, err)
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

// underHome reports whether path is strictly nested under crowbarHome (home as a
// proper directory-boundary prefix). It is the boot-sweep's copy of the delete
// reactor's managedWorktreePath guard — the internal worktreepath package is not
// importable from this package, so the identical strict-prefix test is inlined
// here so both the removed WORKTREE and its removed ROOT parent are proven under
// home before any rm.
func underHome(
	path string,
	crowbarHome string,
) bool {
	if path == "" || crowbarHome == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(crowbarHome, "/")+"/")
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

// reconcileAgentRunners reaps the live-runner rows of the previous run: agent_runners is
// durable sqlite and is never truncated at boot, but a PTY never survives a restart, so
// every row the daemon comes back to describes a CLI that no longer exists. Nothing
// recorded those deaths — the only thing that ever does is an exit callback that lived in
// the process that went away.
//
// A stale row is not cosmetic: it is indistinguishable from a running CLI to every read in
// the agent package, so it BRICKS the chat it names (ResumeChat finds it and no-ops, and
// the pane attaches to a dead terminal session) and, if the chat was mid-turn, spins its
// spinner forever. This runs on EVERY boot, so those states last until this call, not
// until the user gives up.
//
// It runs SYNCHRONOUSLY and AFTER startRestoreTerminalSessions. Synchronously matters: the
// first client read must not race it, or an HTTP read that beats the reconcile is served a
// corpse. The ordering relative to the restore does NOT: the reconcile asks SessionLive,
// which questions the PTY directly, not the terminal registry, so a not-yet-restored session
// and a restored PTY-less placeholder both read false — correctly, since a restored session
// is never live either way. (The hazard that WOULD make this ordering load-bearing belongs
// to the engine's SessionExists, not SessionLive: SessionExists answers true for every
// restored placeholder, which is exactly why ReconcileRunnersOnBoot insists on SessionLive
// instead — see its call site in agent.go.)
//
// Best-effort: a failure is logged and the daemon still boots. The chats it could not
// reconcile are no worse off than they were a line earlier, and refusing to start the
// daemon over them would be strictly worse.
//
// (Its sibling, seedAgentRegistry, is NOT coming back: it rehydrated an in-memory
// session→chat index from persisted segments, and that index is now the runner aggregate's
// append-only conversation history — already durable, and answering ChatForSession straight
// from the read model.)
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
