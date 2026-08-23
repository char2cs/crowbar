package realtime

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

// OriginFetcher is the narrow git surface the origin-sync manager needs. It is
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

// OriginSyncInterval is the cadence of the per-active-WS-connection origin
// check the OriginSyncManager runs for the single subscribed protected
// workspace. Longer than the provider poll's 1-minute cadence since origin
// staleness is far less time-sensitive than PR/CI status.
const OriginSyncInterval = 5 * time.Minute

// originSyncTimeout bounds a single FetchRef invocation. A hung remote
// (network partition, credential prompt on stdin) must not leak the sync
// goroutine forever; WithoutCancel decouples the fetch from the
// per-connection ctx, and this timeout still guarantees forward progress on
// every tick.
const originSyncTimeout = 30 * time.Second

// originSyncHandle tracks one workspace's running origin-sync ticker: its
// subscriber refcount and the cancel func that stops the ticking goroutine.
type originSyncHandle struct {
	refs   int
	cancel context.CancelFunc
}

// OriginSyncManager is the refcounted, lazy lifecycle registry for the
// per-active-WS-connection protected-branch origin check. It mirrors
// ProviderPollManager with its OWN handle map: the first git/status
// subscriber on a single protected workspace starts a goroutine that
// periodically refreshes that workspace's local knowledge of
// origin/<branch>, and the last unsubscribe stops it. A workspace with a
// parent (ParentID != "") is left alone every tick — a child forks from
// origin/<parent> at creation and diffs resolve through origin/<base>, so only
// a protected ROOT needs this. Every tick refreshes that root's origin ref; the
// OPEN-time tick additionally fast-forwards its worktree (advanceLockedRoot).
type OriginSyncManager struct {
	root      context.Context
	interval  time.Duration
	workspace OriginSyncWorkspaces
	git       OriginFetcher
	mu        sync.Mutex
	handles   map[string]*originSyncHandle
	closed    bool

	// runners tracks the live run goroutines. Cancelling a context only ASKS a
	// runner to stop; runners.Wait is the real "it has stopped" signal, which
	// tests block on (waitRunnersForTest) instead of sleeping.
	runners sync.WaitGroup

	// ticks and cycleDone are the deterministic test seams installed by
	// driveCyclesForTest; both are nil in production. See tickSource/notifyCycle.
	ticks     <-chan time.Time
	cycleDone chan<- struct{}
}

// NewOriginSyncManager builds the manager over the per-connection sync
// interval, the workspace lookup, and the git fetch surface. root is the
// parent context for every sync goroutine; cancelling it (via the realtime
// Service) stops all syncs.
func NewOriginSyncManager(
	root context.Context,
	interval time.Duration,
	workspace OriginSyncWorkspaces,
	git OriginFetcher,
) *OriginSyncManager {
	return &OriginSyncManager{
		root:      root,
		interval:  interval,
		workspace: workspace,
		git:       git,
		handles:   make(map[string]*originSyncHandle),
	}
}

// Acquire records one subscriber for wsID. On the 0->1 transition it starts
// the sync goroutine under a cancelable context. A blank wsID is ignored so a
// list-scope (no :wsId) subscribe can never start an unscoped sync. After
// StopAll it is a no-op so a late subscribe from a not-yet-closed hijacked WS
// connection cannot start a sync under the cancelled root.
func (m *OriginSyncManager) Acquire(
	wsID string,
) {
	if wsID == "" {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	if h, ok := m.handles[wsID]; ok {
		h.refs++
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.root)
	m.handles[wsID] = &originSyncHandle{refs: 1, cancel: cancel}
	m.runners.Add(1)
	m.mu.Unlock()

	go m.run(ctx, wsID)
}

// Release drops one subscriber for wsID. On the 1->0 transition it cancels
// the sync goroutine. A blank wsID and an unknown wsID are no-ops.
func (m *OriginSyncManager) Release(
	wsID string,
) {
	if wsID == "" {
		return
	}
	m.mu.Lock()
	h, ok := m.handles[wsID]
	if !ok {
		m.mu.Unlock()
		return
	}
	h.refs--
	if h.refs > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.handles, wsID)
	m.mu.Unlock()

	h.cancel()
}

// StopAll cancels every live sync and clears the handle map. It is idempotent
// and marks the manager closed so subsequent Acquire calls no-op. It runs on
// graceful shutdown (via the realtime Service Close).
func (m *OriginSyncManager) StopAll() {
	m.mu.Lock()
	m.closed = true
	handles := m.handles
	m.handles = make(map[string]*originSyncHandle)
	m.mu.Unlock()

	for _, h := range handles {
		h.cancel()
	}
}

// run ticks every interval and issues a single-workspace origin sync until
// the per-connection context is cancelled. The sync runs on a
// WithoutCancel-derived context so a Release/StopAll mid-tick cannot abort
// an in-flight fetch.
func (m *OriginSyncManager) run(
	ctx context.Context,
	wsID string,
) {
	defer m.runners.Done()
	defer safego.Recover("realtime.originSync.run")
	// Sync once immediately on the 0->1 subscriber transition so a freshly
	// viewed protected workspace's ahead/behind updates within ~1s instead of
	// after a full interval. Skip it only if the subscriber already released
	// mid-startup.
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
func (m *OriginSyncManager) tickSource() (<-chan time.Time, func()) {
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
func (m *OriginSyncManager) notifyCycle(
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

// driveCyclesForTest installs the deterministic test seams; it must be called
// before Acquire. ticks replaces the interval ticker (so no cycle ever happens
// unless the test fires one) and cycleDone receives one value after every
// completed cycle, the immediate-on-Acquire sync included.
func (m *OriginSyncManager) driveCyclesForTest(
	ticks <-chan time.Time,
	cycleDone chan<- struct{},
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticks = ticks
	m.cycleDone = cycleDone
}

// waitRunnersForTest blocks until every run goroutine started by Acquire has
// actually returned. Release/StopAll only cancel a context; this is the real
// signal that the sync has stopped, so a test can assert "nothing fires after
// Release" without a sleep.
func (m *OriginSyncManager) waitRunnersForTest() {
	m.runners.Wait()
}

// syncTick loads the workspace and, only if it has no parent (a protected/
// root branch), issues a single FetchRef on a context.WithoutCancel-derived,
// timeout-bounded context. A workspace with a parent is left alone: its fork
// point is already kept fresh at fork-time by FastForwardBranch. Any failure
// (workspace lookup or fetch) is logged and swallowed — this is best-effort
// background work the user never explicitly requested.
func (m *OriginSyncManager) syncTick(
	ctx context.Context,
	wsID string,
	advance bool,
) {
	m.syncTickWithTimeout(ctx, wsID, originSyncTimeout, advance)
}

// syncTickWithTimeout is syncTick with an injectable timeout (the test seam
// for a short interval). The cancel func runs before this returns so no
// timer outlives the tick.
func (m *OriginSyncManager) syncTickWithTimeout(
	ctx context.Context,
	wsID string,
	timeout time.Duration,
	advance bool,
) {
	syncCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
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
		slog.WarnContext(syncCtx, "origin sync: fetch ref", "workspace_id", wsID, "branch", ws.Branch, "err", err)
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
// It runs ONLY on the open (0->1 subscriber) sync, never on an interval tick.
// The cadence is the reason: the sync ticks only while a client is subscribed to
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
func (m *OriginSyncManager) advanceLockedRoot(
	ctx context.Context,
	ws domain.Workspace,
) {
	if ws.Status != domain.WorkspaceStatusLocked || ws.IsDefault || ws.WorktreePath == "" {
		return
	}
	err := m.git.MergeFFOnly(ctx, ws.WorktreePath, "origin/"+ws.Branch)
	if err == nil {
		return
	}
	if errors.Is(err, enginegit.ErrDirtyTree) {
		slog.InfoContext(ctx, "origin sync: locked root has local changes; leaving it where it is",
			"workspace_id", ws.ID, "branch", ws.Branch)
		return
	}
	slog.WarnContext(ctx, "origin sync: could not fast-forward locked root",
		"workspace_id", ws.ID, "branch", ws.Branch, "err", err)
}
