package resources

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// OriginSyncWorkspaces is the narrow workspace-lookup surface the origin-sync
// manager needs. It is satisfied by workspacerepo.Workspace so the manager
// stays decoupled from the concrete repository and is trivially fakeable in
// tests.
type OriginSyncWorkspaces interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// OriginFetcher is the narrow git surface the origin sync needs. It is
// satisfied by engine/git.Engine so the manager stays decoupled from the
// concrete engine and is trivially fakeable in tests.
type OriginFetcher interface {
	FetchRef(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
	// MergeFFOnly runs `git merge --ff-only <ref>` in repoPath. It is git — not
	// this manager — that decides whether the advance is safe: it refuses rather
	// than overwrite local changes, so no pre-flight cleanliness check is needed
	// (and none would be right: `git status` counts untracked files, so a locked
	// worktree holding build output would never be considered clean).
	MergeFFOnly(
		ctx context.Context,
		repoPath string,
		branch string,
	) error
}

// OriginSyncInterval is the cadence of the origin check OriginSync runs for
// a held protected workspace. Longer than the provider poll's 1-minute cadence since origin
// staleness is far less time-sensitive than PR/CI status.
const OriginSyncInterval = 5 * time.Minute

// originSyncTimeout bounds a single FetchRef invocation: a hung remote (network
// partition, credential prompt on stdin) must not wedge the sync goroutine. A
// Stop or Close cancels the fetch sooner, terminating the git process.
const originSyncTimeout = 30 * time.Second

// OriginSync keeps a held protected root's knowledge of origin/<branch> fresh
// while a client holds its git: scope: once at Start, which also fast-forwards
// a locked root (advanceLockedRoot), then every interval. A workspace with a
// parent is left alone: a child forks from origin/<parent> and diffs resolve
// through origin/<base>, so only a protected ROOT needs this.
type OriginSync struct {
	root      context.Context
	interval  time.Duration
	workspace OriginSyncWorkspaces
	git       OriginFetcher
	mu        sync.Mutex
	cancels   map[string]context.CancelFunc
	closed    bool

	// runners tracks the live run goroutines. Cancelling a context only ASKS a
	// runner to stop; runners.Wait is the real "it has stopped" signal, which
	// Close joins on and tests block on after Stop (waitRunnersForTest).
	runners sync.WaitGroup

	// ticks and cycleDone are the deterministic test seams installed by
	// driveCyclesForTest; both are nil in production. See tickSource/notifyCycle.
	ticks     <-chan time.Time
	cycleDone chan<- struct{}
}

// NewOriginSync builds the resource over the sync interval, the workspace
// lookup and the git fetch surface. root is the parent context for every sync.
func NewOriginSync(
	root context.Context,
	interval time.Duration,
	workspace OriginSyncWorkspaces,
	git OriginFetcher,
) *OriginSync {
	return &OriginSync{
		root:      root,
		interval:  interval,
		workspace: workspace,
		git:       git,
		cancels:   make(map[string]context.CancelFunc),
	}
}

// Start begins syncing wsID. After Close it refuses, so a late hold cannot
// start a sync under the cancelled root.
func (m *OriginSync) Start(
	wsID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errClosed
	}
	ctx, cancel := context.WithCancel(m.root)
	m.cancels[wsID] = cancel
	m.runners.Add(1)
	go m.run(ctx, wsID)
	return nil
}

// Stop cancels wsID's sync, a fetch in flight included.
func (m *OriginSync) Stop(
	wsID string,
) {
	m.mu.Lock()
	cancel, ok := m.cancels[wsID]
	delete(m.cancels, wsID)
	m.mu.Unlock()
	if ok {
		cancel()
	}
}

// Close cancels every live sync and joins every runner, so no fetch outlives
// shutdown into a closed store or a removed repo. It is idempotent and
// refuses later starts.
func (m *OriginSync) Close() {
	m.mu.Lock()
	m.closed = true
	cancels := m.cancels
	m.cancels = make(map[string]context.CancelFunc)
	m.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	m.runners.Wait()
}

// run ticks every interval and issues a single-workspace origin sync until
// the per-connection context is cancelled. The sync runs under that context,
// so Stop and Close cancel a fetch in flight instead of waiting it out.
func (m *OriginSync) run(
	ctx context.Context,
	wsID string,
) {
	defer m.runners.Done()
	defer safego.Recover("resources.originSync.run")
	// Sync once immediately on Start so a freshly viewed protected workspace's
	// ahead/behind updates within ~1s instead of after a full interval. Skip it
	// only if the hold already ended mid-startup.
	select {
	case <-ctx.Done():
		return
	default:
		m.syncTick(ctx, wsID, true)
		m.notifyCycle(ctx)
	}

	tickC, stopTicker := m.tickSource()
	defer stopTicker()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			m.syncTick(ctx, wsID, false)
			m.notifyCycle(ctx)
		}
	}
}

// tickSource returns the cadence channel run listens on, plus its stop func. In
// production that is a real interval ticker. A test installs its own source
// (driveCyclesForTest) so a cycle happens exactly when the test fires one and the
// background cadence can never race the test — the same discipline as
// core/terminal's StopMaintenanceForTest + RunMaintenanceOnceForTest. Sleeping
// for a 5-minute production interval is not an option, and shrinking the interval
// to "small enough" would only trade a slow test for a flaky one.
func (m *OriginSync) tickSource() (<-chan time.Time, func()) {
	m.mu.Lock()
	ticks := m.ticks
	m.mu.Unlock()

	if ticks != nil {
		return ticks, func() {}
	}
	ticker := time.NewTicker(m.interval)
	return ticker.C, ticker.Stop
}

// notifyCycle signals the installed test seam that one full sync cycle has
// finished (a fetch, or a deliberate skip). It is the REAL end-of-cycle signal a
// test blocks on to assert that NOTHING happened — an assertion that would
// otherwise need a sleep, which is a guess. It is a no-op in production, and it
// gives up on ctx cancellation so a runner can never be wedged by it.
func (m *OriginSync) notifyCycle(
	ctx context.Context,
) {
	m.mu.Lock()
	done := m.cycleDone
	m.mu.Unlock()

	if done == nil {
		return
	}
	select {
	case done <- struct{}{}:
	case <-ctx.Done():
	}
}

// syncTick loads the workspace and, only if it has no parent (a protected/
// root branch), issues a single FetchRef bounded by originSyncTimeout. A
// workspace with a parent is left alone: its fork point is already kept fresh
// at fork-time by FastForwardBranch. Any failure (workspace lookup or fetch) is
// logged and swallowed — this is best-effort background work the user never
// explicitly requested.
func (m *OriginSync) syncTick(
	ctx context.Context,
	wsID string,
	advance bool,
) {
	m.syncTickWithTimeout(ctx, wsID, originSyncTimeout, advance)
}

// syncTickWithTimeout is syncTick with an injectable timeout (the test seam
// for a short interval). The cancel func runs before this returns so no
// timer outlives the tick.
func (m *OriginSync) syncTickWithTimeout(
	ctx context.Context,
	wsID string,
	timeout time.Duration,
	advance bool,
) {
	syncCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ws, err := m.workspace.Get(syncCtx, wsID)
	if err != nil {
		slog.WarnContext(syncCtx, "origin sync: load workspace", "workspace_id", wsID, "err", err)
		return
	}
	if ws.ParentID != "" {
		return
	}
	if err := m.git.FetchRef(syncCtx, ws.WorktreePath, ws.Branch); err != nil {
		slog.DebugContext(syncCtx, "origin sync: fetch ref", "workspace_id", wsID, "branch", ws.Branch, "err", err)
		return
	}
	if advance {
		m.advanceLockedRoot(syncCtx, ws)
	}
}

// advanceLockedRoot fast-forwards a locked root's worktree onto the origin ref
// the caller has just fetched, so the files the user is about to read are the
// files origin has.
//
// It runs ONLY on the open (Start) sync, never on an interval tick.
// The cadence is the reason: the sync ticks only while a client holds
// this workspace — that is, only while the user is looking at it — so advancing
// on a tick would move files under a live editor or a mid-task agent, and would
// do nothing during the entire window when advancing is harmless. Doing it once,
// at open, puts the change before the reading rather than during it.
//
// Only a LOCKED, non-default, provisioned root qualifies. Locked is what makes a
// fast-forward unconditionally safe: Crowbar refuses commits and merges into
// these branches, so there is no local work for origin to race. The repo home is
// excluded because it is the user's own folder, which Crowbar does not own, and
// a placeholder (empty WorktreePath) has no worktree to advance.
//
// Every failure is swallowed: an unreachable remote, or a tree dirtied from a
// terminal (git refuses rather than overwrite), must never break opening a
// workspace. A refusal is expected and logged quietly; anything else is a Warn.
func (m *OriginSync) advanceLockedRoot(
	ctx context.Context,
	ws domain.Workspace,
) {
	if ws.Status != domain.WorkspaceStatusLocked || ws.Provisioning != domain.WorkspaceProvisioned {
		return
	}
	// A cancel decides only whether the fast-forward starts: killed mid-checkout,
	// git leaves the tree half-advanced and refuses every later fast-forward.
	if ctx.Err() != nil {
		return
	}
	err := m.git.MergeFFOnly(context.WithoutCancel(ctx), ws.WorktreePath, "origin/"+ws.Branch)
	if err == nil {
		return
	}
	if errors.Is(err, enginegit.ErrDirtyTree) {
		slog.DebugContext(ctx, "origin sync: locked root has local changes; leaving it where it is",
			"workspace_id", ws.ID, "branch", ws.Branch)
		return
	}
	slog.WarnContext(ctx, "origin sync: could not fast-forward locked root",
		"workspace_id", ws.ID, "branch", ws.Branch, "err", err)
}
