package realtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
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
// parent (ParentID != "") is left alone every tick — its fork point is
// already kept fresh by the fork-time FastForwardBranch, and this manager
// exists only to un-stale a protected root branch's ahead/behind display.
type OriginSyncManager struct {
	root      context.Context
	interval  time.Duration
	workspace OriginSyncWorkspaces
	git       OriginFetcher
	mu        sync.Mutex
	handles   map[string]*originSyncHandle
	closed    bool
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
	defer safego.Recover("realtime.originSync.run")
	// Sync once immediately on the 0->1 subscriber transition so a freshly
	// viewed protected workspace's ahead/behind updates within ~1s instead of
	// after a full interval. Skip it only if the subscriber already released
	// mid-startup.
	select {
	case <-ctx.Done():
		return
	default:
		m.syncTick(ctx, wsID)
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncTick(ctx, wsID)
		}
	}
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
) {
	m.syncTickWithTimeout(ctx, wsID, originSyncTimeout)
}

// syncTickWithTimeout is syncTick with an injectable timeout (the test seam
// for a short interval). The cancel func runs before this returns so no
// timer outlives the tick.
func (m *OriginSyncManager) syncTickWithTimeout(
	ctx context.Context,
	wsID string,
	timeout time.Duration,
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
	}
}
