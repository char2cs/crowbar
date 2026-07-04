# Protected Branch Origin Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep a protected/root workspace's `ahead`/`behind` git status honest against `origin` by periodically fetching (never pulling) its branch while its git-status WebSocket is subscribed, so the already-existing "Clean · N behind → Pull N" UI shows real staleness instead of never refreshing after initial provisioning.

**Architecture:** A new refcounted `OriginSyncManager` in the realtime layer, structurally mirroring the existing `ProviderPollManager`: the first `git/status` WS subscriber for a workspace starts a goroutine that ticks every 5 minutes (syncing immediately on start too); each tick loads the workspace and, only if it has no parent (`ParentID == ""`), does a best-effort `FetchRef` (single-branch fetch, remote-tracking ref only — never the working tree or local branch ref). The last unsubscribe stops the goroutine. Wired onto the existing `git` WebSocket broadcaster's `OnSubscribe`/`OnUnsubscribe` hooks, chained alongside the existing watcher-acquire behavior. No new REST endpoint, no frontend change — the already-existing file watcher (which already watches `.git/refs/*`/`packed-refs`) picks up the fetch's ref update and pushes fresh status over the same already-open socket.

**Tech Stack:** Go backend (`api/internal/app/realtime`, `api/internal/api/v0`), no frontend changes, no new dependencies.

## Global Constraints

- Fetch-only, never pull/fast-forward: staleness must stay visible and actionable via the existing Pull button, not silently self-heal.
- Scope: protected/no-parent workspaces only (`domain.Workspace.ParentID == ""`). Workspaces with a parent are never touched by this feature.
- Poll interval: `5 * time.Minute`, a hardcoded constant (not user-configurable) — named `realtime.OriginSyncInterval`.
- Fetch primitive: `enginegit.Engine.FetchRef(ctx, repoPath, branch)` (single branch), not the blanket `Fetch()`.
- Hook point: the existing `git/status` WebSocket subscribe/unsubscribe lifecycle only — no new REST endpoint, no frontend change.
- All background work is best-effort and silent: errors are logged (`slog.WarnContext`) and swallowed, never surfaced to the user.
- Every per-tick network call runs on a `context.WithoutCancel`-derived context bounded by a 30-second timeout (`originSyncTimeout`), so a hung fetch cannot leak a goroutine or block shutdown.
- Backend unit tests: `cd api && go test -tags noEmbed -race ./...`. Backend integration tests: `cd api && go test -tags 'integration noEmbed' -race -v -timeout 600s -p 1 ./...`.
- Spec: `docs/superpowers/specs/2026-07-04-protected-branch-origin-sync-design.md`.

---

## File Structure

- **Create:** `api/internal/app/realtime/origin_sync_manager.go` — the `OriginSyncManager`, its narrow `OriginSyncWorkspaces`/`OriginFetcher` interfaces, and the `OriginSyncInterval` constant.
- **Create:** `api/internal/app/realtime/origin_sync_manager_test.go` — full test suite, mirroring `provider_poll_manager_test.go`.
- **Modify:** `api/internal/app/realtime/service.go` — add the `originSync` field, construct it in `New`, add `AcquireOriginSync`/`ReleaseOriginSync`, wire into `Close`.
- **Modify:** `api/internal/app/container.go:80-90` — pass `realtime.OriginSyncInterval` into the single `realtime.New(...)` call site.
- **Modify:** `api/internal/api/v0/container.go` — add the `withOriginSyncLifecycle` decorator and apply it to the `git:` broadcaster construction.

---

### Task 1: `OriginSyncManager`

**Files:**
- Create: `api/internal/app/realtime/origin_sync_manager.go`
- Create: `api/internal/app/realtime/origin_sync_manager_test.go`

**Interfaces:**
- Produces: `realtime.OriginSyncWorkspaces` (interface: `Get(ctx, id string) (domain.Workspace, error)`), `realtime.OriginFetcher` (interface: `FetchRef(ctx, repoPath, branch string) error`), `realtime.OriginSyncInterval` (const, `5 * time.Minute`), `realtime.NewOriginSyncManager(root context.Context, interval time.Duration, workspace OriginSyncWorkspaces, git OriginFetcher) *OriginSyncManager`, and methods `(*OriginSyncManager).Acquire(wsID string)`, `.Release(wsID string)`, `.StopAll()`. These are consumed by Task 2.

- [ ] **Step 1: Write the failing test file**

Create `api/internal/app/realtime/origin_sync_manager_test.go`:

```go
package realtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeOriginWorkspaces is an in-memory OriginSyncWorkspaces fake keyed by
// workspace ID, safe for concurrent Get/set from the manager's goroutines and
// the test's main goroutine.
type fakeOriginWorkspaces struct {
	mu   sync.Mutex
	byID map[string]domain.Workspace
}

func newFakeOriginWorkspaces() *fakeOriginWorkspaces {
	return &fakeOriginWorkspaces{byID: make(map[string]domain.Workspace)}
}

func (f *fakeOriginWorkspaces) set(
	ws domain.Workspace,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[ws.ID] = ws
}

func (f *fakeOriginWorkspaces) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ws, ok := f.byID[id]
	if !ok {
		return domain.Workspace{}, fmt.Errorf("workspace %s not found", id)
	}
	return ws, nil
}

// fakeOriginFetcher records every FetchRef call on a channel so tests can
// synchronise on the per-connection ticker without sleeping.
type fakeOriginFetcher struct {
	calls chan string
}

func newFakeOriginFetcher() *fakeOriginFetcher {
	return &fakeOriginFetcher{calls: make(chan string, 64)}
}

func (f *fakeOriginFetcher) FetchRef(
	_ context.Context,
	repoPath string,
	branch string,
) error {
	f.calls <- repoPath + ":" + branch
	return nil
}

const testOriginSyncInterval = time.Millisecond

func protectedWorkspace(
	id string,
) domain.Workspace {
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "develop"}
}

func childWorkspace(
	id string,
) domain.Workspace {
	return domain.Workspace{ID: id, WorktreePath: "/repo/" + id, Branch: "feature", ParentID: "parent-1"}
}

func TestOriginSyncManager_Acquire_SyncsProtectedWorkspace(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	select {
	case got := <-f.calls:
		assert.Equal(t, "/repo/w1:develop", got)
	case <-time.After(time.Second):
		t.Fatal("FetchRef was not invoked after Acquire for a protected workspace")
	}
}

func TestOriginSyncManager_Acquire_SkipsWorkspaceWithParent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(childWorkspace("w2"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w2")

	select {
	case <-f.calls:
		t.Fatal("FetchRef fired for a workspace with a parent")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_Acquire_SyncsImmediately(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	// A long interval ensures the ticker cannot fire within the test window, so
	// observing a fetch proves the immediate-on-Acquire sync, not a ticker tick.
	m := NewOriginSyncManager(context.Background(), time.Hour, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")

	select {
	case got := <-f.calls:
		assert.Equal(t, "/repo/w1:develop", got)
	case <-time.After(time.Second):
		t.Fatal("Acquire did not sync immediately (waited for the interval)")
	}
}

func TestOriginSyncManager_Release_StopsSync(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	<-f.calls // confirm syncing started
	m.Release("w1")

	drainOriginCalls(f.calls)

	select {
	case <-f.calls:
		t.Fatal("sync fired after Release stopped the workspace")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_Refcount(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("w1")
	m.Acquire("w1")
	<-f.calls
	m.Release("w1") // refs 2 -> 1, still syncing

	drainOriginCalls(f.calls)

	select {
	case got := <-f.calls:
		assert.Equal(t, "/repo/w1:develop", got)
	case <-time.After(time.Second):
		t.Fatal("sync stopped despite an outstanding subscriber")
	}
}

func TestOriginSyncManager_BlankWsID_NoOp(t *testing.T) {
	w := newFakeOriginWorkspaces()
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	m.Acquire("")
	require.NotPanics(t, func() { m.Release("") })
	require.NotPanics(t, func() { m.Release("never-acquired") })

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	select {
	case <-f.calls:
		t.Fatal("blank wsID started a sync")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_StopAll_Idempotent(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)

	m.Acquire("w1")
	<-f.calls

	require.NotPanics(t, m.StopAll)
	require.NotPanics(t, m.StopAll) // second call is safe

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestOriginSyncManager_AcquireAfterClose_NoOp(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)

	m.StopAll()
	m.Acquire("w1")

	m.mu.Lock()
	count := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, count)

	select {
	case <-f.calls:
		t.Fatal("Acquire after StopAll started a sync")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOriginSyncManager_SyncTick_WorkspaceLookupError_Swallowed(t *testing.T) {
	w := newFakeOriginWorkspaces() // no workspace seeded -> Get errors
	f := newFakeOriginFetcher()
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, f)
	t.Cleanup(m.StopAll)

	require.NotPanics(t, func() { m.Acquire("missing") })

	select {
	case <-f.calls:
		t.Fatal("FetchRef called despite a workspace lookup error")
	case <-time.After(20 * time.Millisecond):
	}
}

// blockingFetcher blocks each FetchRef on the per-fetch context until that
// context is cancelled, then reports the cancellation cause on a channel. It
// proves the manager bounds a hung fetch: a remote that never returns must not
// wedge the run goroutine forever.
type blockingFetcher struct {
	released chan error
}

func newBlockingFetcher() *blockingFetcher {
	return &blockingFetcher{released: make(chan error, 8)}
}

func (b *blockingFetcher) FetchRef(
	ctx context.Context,
	_ string,
	_ string,
) error {
	<-ctx.Done()
	b.released <- ctx.Err()
	return ctx.Err()
}

func TestOriginSyncManager_SyncTick_CancelsHungFetch(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	b := newBlockingFetcher()
	m := NewOriginSyncManager(context.Background(), time.Hour, w, b)

	// Drive a single tick directly with a short injected timeout so the test
	// does not wait the production 30s. The fetch blocks on ctx.Done(); the
	// timeout must fire and unblock it, proving a hung remote cannot wedge the
	// run goroutine.
	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		m.syncTickWithTimeout(ctx, "w1", 10*time.Millisecond)
		close(done)
	}()

	select {
	case err := <-b.released:
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"hung fetch must be cancelled by the per-sync timeout")
	case <-time.After(time.Second):
		t.Fatal("fetch was never cancelled — the per-sync timeout did not fire")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("syncTick did not return after the fetch was cancelled")
	}
}

func TestOriginSyncManager_SyncTick_DecoupledFromConnCtx(t *testing.T) {
	w := newFakeOriginWorkspaces()
	w.set(protectedWorkspace("w1"))
	b := newBlockingFetcher()
	m := NewOriginSyncManager(context.Background(), time.Hour, w, b)

	// The per-connection ctx is ALREADY cancelled, yet WithoutCancel must let
	// the fetch start (it only stops via its own timeout). This preserves the
	// best-effort background-sync guarantee while still bounding the fetch.
	connCtx, cancelConn := context.WithCancel(context.Background())
	cancelConn()

	go m.syncTickWithTimeout(connCtx, "w1", 10*time.Millisecond)

	select {
	case err := <-b.released:
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"fetch must run under WithoutCancel and stop only on its own timeout")
	case <-time.After(time.Second):
		t.Fatal("fetch did not start/cancel despite an already-cancelled conn ctx")
	}
}

// drainOriginCalls empties any already-buffered calls so a subsequent receive
// observes only post-drain activity.
func drainOriginCalls(
	c chan string,
) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

// ensure the fakes satisfy their interfaces at compile time.
var (
	_ OriginSyncWorkspaces = (*fakeOriginWorkspaces)(nil)
	_ OriginFetcher        = (*fakeOriginFetcher)(nil)
	_ OriginFetcher        = (*blockingFetcher)(nil)
)
```

- [ ] **Step 2: Run the test file to verify it fails to compile**

Run: `cd api && go test -tags noEmbed ./internal/app/realtime/... -run TestOriginSyncManager`
Expected: FAIL — compile error, e.g. `undefined: OriginSyncWorkspaces` / `undefined: NewOriginSyncManager` (the production file doesn't exist yet).

- [ ] **Step 3: Write the manager implementation**

Create `api/internal/app/realtime/origin_sync_manager.go`:

```go
package realtime

import (
	"context"
	"log/slog"
	"sync"
	"time"

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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd api && go test -tags noEmbed -race ./internal/app/realtime/... -run TestOriginSyncManager -v`
Expected: PASS — all 11 `TestOriginSyncManager_*` tests pass.

- [ ] **Step 5: Commit**

```bash
cd api
git add internal/app/realtime/origin_sync_manager.go internal/app/realtime/origin_sync_manager_test.go
git commit -m "feat(realtime): add refcounted OriginSyncManager for protected branches"
```

---

### Task 2: Wire `OriginSyncManager` into `realtime.Service`

**Files:**
- Modify: `api/internal/app/realtime/service.go`
- Modify: `api/internal/app/container.go:80-90`

**Interfaces:**
- Consumes: `realtime.NewOriginSyncManager`, `realtime.OriginSyncInterval`, `(*OriginSyncManager).Acquire/Release/StopAll` (Task 1).
- Produces: `(*realtime.Service).AcquireOriginSync(wsID string)`, `(*realtime.Service).ReleaseOriginSync(wsID string)` — consumed by Task 3.

- [ ] **Step 1: Modify `service.go` — add the field, constructor wiring, methods, and shutdown hook**

In `api/internal/app/realtime/service.go`, change the `Service` struct (around line 27):

```go
type Service struct {
	watchers     *WatcherManager
	lsps         *LSPManager
	providerPoll *ProviderPollManager
	originSync   *OriginSyncManager
	cancel       context.CancelFunc
	closeOnce    sync.Once
}
```

Change `New`'s signature and body (around lines 40-58) to:

```go
// New builds the realtime Service from the hub, the workspace repository, the
// git and fs engines, the LSP lifecycle, the provider-poll usecase, the
// per-connection poll interval, the origin-sync interval, and a clock. It
// derives a cancelable child of ctx so Close can stop every watcher, LSP
// host, provider poll, and origin sync on graceful shutdown; New must not
// block, so the goroutines are started lazily on Acquire.
func New(
	ctx context.Context,
	h *hub.Hub,
	workspace workspacerepo.Workspace,
	gitEngine enginegit.Engine,
	fsEngine enginefs.Engine,
	lspLifecycle LSPLifecycle,
	providerPoll ProviderPoller,
	perConnPollInterval time.Duration,
	originSyncInterval time.Duration,
	now func() time.Time,
) *Service {
	root, cancel := context.WithCancel(ctx)
	return &Service{
		watchers:     NewWatcherManager(root, h, workspace, gitEngine, fsEngine, now),
		lsps:         NewLSPManager(root, lspLifecycle),
		providerPoll: NewProviderPollManager(root, perConnPollInterval, providerPoll),
		originSync:   NewOriginSyncManager(root, originSyncInterval, workspace, gitEngine),
		cancel:       cancel,
	}
}
```

Add two new methods after `ReleaseProviderPoll` (around line 107):

```go
// AcquireOriginSync records one git/status WS subscriber for wsID, starting
// the 5-minute per-connection protected-branch origin check on the 0->1
// transition. A blank wsID (the workspace list scope) no-ops in the manager.
func (s *Service) AcquireOriginSync(
	wsID string,
) {
	s.originSync.Acquire(wsID)
}

// ReleaseOriginSync drops one git/status WS subscriber for wsID, stopping the
// per-connection origin check on the 1->0 transition.
func (s *Service) ReleaseOriginSync(
	wsID string,
) {
	s.originSync.Release(wsID)
}
```

Change `Close` (around line 113) to also stop the new manager:

```go
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.watchers.StopAll()
		s.lsps.StopAll()
		s.providerPoll.StopAll()
		s.originSync.StopAll()
	})
}
```

- [ ] **Step 2: Update the single call site in `api/internal/app/container.go`**

In `api/internal/app/container.go`, change the `realtime.New(...)` call (around lines 80-90) from:

```go
	rt := realtime.New(
		ctx,
		h,
		repos.Workspace,
		engines.Git,
		engines.FS,
		realtime.NewLSPLifecycle(engines.LSP),
		ucs.ProviderSync,
		poll.PerConnectionInterval,
		time.Now,
	)
```

to:

```go
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
```

- [ ] **Step 3: Confirm it builds and every existing realtime/app test still passes**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed -race ./internal/app/... -v`
Expected: PASS — the build succeeds (confirming `realtime.New`'s only call site was updated correctly) and every existing test in `internal/app/...` (including all of `internal/app/realtime`) still passes unchanged.

- [ ] **Step 4: Commit**

```bash
cd api
git add internal/app/realtime/service.go internal/app/container.go
git commit -m "feat(realtime): wire OriginSyncManager into the realtime Service"
```

---

### Task 3: Wire the origin-sync trigger onto the `git` WebSocket broadcaster

**Files:**
- Modify: `api/internal/api/v0/container.go`

**Interfaces:**
- Consumes: `(*realtime.Service).AcquireOriginSync`, `.ReleaseOriginSync` (Task 2).

- [ ] **Step 1: Add the `withOriginSyncLifecycle` decorator**

In `api/internal/api/v0/container.go`, add this function after `withLSPLifecycle` (around line 182, before `withProviderPollLifecycle`):

```go
// withOriginSyncLifecycle attaches the protected-branch origin-sync trigger
// to a StreamDef, scoping by wsId resolved from the path or query and
// delegating to the app-layer realtime service. It CHAINS onto whatever
// OnSubscribe/OnUnsubscribe the StreamDef already carries (e.g.
// withWatcherLifecycle's watcher acquire) rather than replacing them, so both
// fire on every subscribe/unsubscribe.
func withOriginSyncLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	prevSubscribe := def.OnSubscribe
	prevUnsubscribe := def.OnUnsubscribe
	def.ScopeKey = scopeWsID
	def.OnSubscribe = func(scope string) {
		if prevSubscribe != nil {
			prevSubscribe(scope)
		}
		appContainer.Realtime.AcquireOriginSync(scope)
	}
	def.OnUnsubscribe = func(scope string) {
		if prevUnsubscribe != nil {
			prevUnsubscribe(scope)
		}
		appContainer.Realtime.ReleaseOriginSync(scope)
	}
	return def
}
```

- [ ] **Step 2: Apply it to the `git` broadcaster construction**

In the same file, change the `git:` line in `New` (around line 70) from:

```go
		git:        ws.NewBroadcaster(withWatcherLifecycle(gitDef(appContainer), appContainer)),
```

to:

```go
		git:        ws.NewBroadcaster(withOriginSyncLifecycle(withWatcherLifecycle(gitDef(appContainer), appContainer), appContainer)),
```

- [ ] **Step 3: Run the existing integration suite to confirm the git WS path still works unchanged**

Run: `cd api && go test -tags 'integration noEmbed' -race -v -p 1 ./internal/api/v0/... -run 'TestV0_PushGit|TestV0_GitDualServe'`
Expected: PASS — `TestV0_PushGit_QueryScope_IsolatesWsId` and `TestV0_GitDualServe_PathScope_IsolatesWsId` both still pass, confirming the new chained `OnSubscribe`/`OnUnsubscribe` composition doesn't disturb the existing git WS fan-out/scoping behavior.

- [ ] **Step 4: Run the full backend build + unit suite one more time**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed -race ./...`
Expected: PASS — full unit build and test suite green.

- [ ] **Step 5: Commit**

```bash
cd api
git add internal/api/v0/container.go
git commit -m "feat(api): trigger origin-sync on git/status WS subscribe for protected branches"
```

- [ ] **Step 6: Manual verification in the running Tauri app**

Per the project's standing rule (verify in Tauri before claiming a UI-adjacent change works): open a protected branch's workspace tab (e.g. `develop`) in the real running app. Confirm via daemon logs (`/tmp/crowbar-daemon.log` or the dev daemon's stdout) that a `git fetch origin develop` (or equivalent `FetchRef` log line, if one exists) runs shortly after the tab opens, and again roughly every 5 minutes while it stays open. If `develop` is genuinely behind `origin/develop` at the time, confirm the `BranchSection` card now shows "Clean · N behind" with a "Pull N" button, and that clicking Pull resolves it. Closing the tab (or switching to a different workspace) should stop further fetches for that workspace.

---

## Self-Review Notes

- **Spec coverage:** Every section of `2026-07-04-protected-branch-origin-sync-design.md` maps to a task — Architecture/Components → Tasks 1-3; Data flow → Task 3 Step 6 (manual verification traces the exact flow); Error handling → `originSyncTimeout` + swallow-and-log in Task 1; Testing → Task 1's full suite + Task 2/3's regression runs + Task 3 Step 6; Out of scope items are not implemented by any task (confirmed no task touches non-protected workspaces, adds a pull/fast-forward, or adds a config setting).
- **Placeholder scan:** No TBD/TODO markers; every step has complete, runnable code or an exact command with an expected result.
- **Type consistency:** `OriginSyncWorkspaces.Get` / `OriginFetcher.FetchRef` signatures are identical everywhere they appear (Task 1's interfaces, implementation, and tests). `NewOriginSyncManager(root, interval, workspace, git)` parameter order and types match every call site (tests in Task 1, `service.go` in Task 2). `AcquireOriginSync`/`ReleaseOriginSync` names and signatures match between Task 2 (defined) and Task 3 (consumed).
