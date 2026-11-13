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

// OriginSyncWorkspaces is the workspace lookup the origin-sync manager needs,
// satisfied by workspacerepo.Workspace.
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
	// MergeFFOnly runs `git merge --ff-only <ref>` in repoPath. git itself refuses to
	// overwrite local changes, so no pre-flight check is needed (git status would also
	// count untracked build output as dirty).
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

// OriginSync keeps a held protected root's origin/<branch> fresh while a client holds
// its git: scope: once at Start (also fast-forwarding a locked root), then every
// interval. A child needs none: it forks from and diffs through its parent's origin ref.
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
	go m.run(ctx, cancel, wsID)
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
	cancel context.CancelFunc,
	wsID string,
) {
	defer m.runners.Done()
	defer cancel()
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

// tickSource returns the cadence channel run listens on and its stop func: a real
// ticker in production, or the test's own source so cycles happen only when fired.
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

// notifyCycle signals the test seam that a sync cycle (fetch or skip) finished, so a
// test can assert nothing happened without sleeping. No-op in production; gives up on ctx.
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

// syncTick fetches the workspace's origin ref, bounded by originSyncTimeout, only if
// it is a parentless root; a child's fork point is kept fresh by FastForwardBranch.
// Failures are logged and swallowed: this is unrequested background work.
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

// advanceLockedRoot fast-forwards a locked, non-default, provisioned root onto the
// just-fetched origin ref. Only at Start, so files never move under a live reader;
// locked means no local work to race. Failures (dirty tree, no remote) are swallowed.
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
