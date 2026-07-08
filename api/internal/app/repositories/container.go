package repositories

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	ReviewThread reviewthread.ReviewThread
	hub          hub.WebSocketHub
	git          wsusecase.MergeConflictChecker
	// inflight counts the background mutations currently running per workspace
	// id (00 §4 fail-fast/good-path-async). It backs the derived Working overlay:
	// the API layer brackets each async op with BeginWork/EndWork, and every
	// serving path (live broadcast, snapshot, REST reads) overlays IsWorking so
	// the client spinner tracks real daemon activity.
	mu       sync.Mutex
	inflight map[string]int

	// drainWG tracks every post-commit reactor goroutine wireCallbacks registered
	// (the async delete reactor, and — once wired — the reconcile-on-open tasks) so
	// the app layer's ordered graceful shutdown (Task 15) can wait them out before
	// closing the DBs; drainCancel closes the shared drain gate to stop reactors
	// starting new work, derived from drainCtx (decisions 9 + 11). They are created
	// and stored by wireCallbacks and reached from app.Container via Drain().
	drainWG     *sync.WaitGroup
	drainCtx    context.Context
	drainCancel context.CancelFunc
}

// ReactorDrain is the shared shutdown handle for every post-commit reactor
// wireCallbacks registered. The app layer's ordered graceful shutdown (Task 15)
// closes the gate (Cancel) so reactors stop starting new work, then waits on WG
// (bounded by the shutdown deadline) before the adapter closes the DBs. Ctx is the
// cancelable parent the gate is derived from (decisions 9 + 11).
type ReactorDrain struct {
	Ctx    context.Context
	WG     *sync.WaitGroup
	Cancel context.CancelFunc
}

// New builds all aggregate repositories, wiring each projection's broadcast into
// the hub. The workspace aggregate is backed by the singleton axWorkspace (one
// instance per type, routing every id by shard hash) built by the app layer; its
// read model lives in state/store/workspace.db and its id↔path index in view.db.
// The reviewthread aggregate owns its central per-type read model at
// state/store/review_thread.db.
func New(
	ctx context.Context,
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axReviewThread asynx.Asynx[domain.ReviewThread],
	axWorkspace asynx.Asynx[domain.Workspace],
	git wsusecase.MergeConflictChecker,
) (*Container, error) {
	c := &Container{hub: h, git: git, inflight: map[string]int{}}
	pathsStore, err := wspaths.NewWorkspacePaths(adapters.GlobalView())
	if err != nil {
		return nil, fmt.Errorf("repositories: workspace paths: %w", err)
	}
	ws, err := workspace.New(axWorkspace, adapters.WorkspaceES(), adapters.WorkspaceView(), pathsStore)
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	// Register the hub (WS fan-out) projection on the singleton axWorkspace,
	// injecting the container-owned enrichment: every event-driven frame goes
	// through the SAME enrichFrame + hub.BroadcastWorkspace as the BeginWork/EndWork
	// rebroadcasts, so the FE spinner + merge badges survive the store/hub split
	// with zero regression (spec §3.5 hub-frame enrichment). The save-only store
	// projection is registered inside workspace.New; the two derive independently
	// from evt.Aggregate and cannot drift (decision 5).
	if err := workspace.RegisterHubProjection(axWorkspace, c.enrichFrame, c.hub.BroadcastWorkspace); err != nil {
		return nil, fmt.Errorf("repositories: workspace hub projection: %w", err)
	}
	// reviewthread owns its own central per-type read model at
	// state/store/review_thread.db (Task 12), no longer the shared view.db: pass
	// ReviewThreadView() as the read-model DB while keeping ReviewThreadES() for the
	// lazy AggregateLister Replay (§3.7).
	rt, err := reviewthread.New(axReviewThread, adapters.ReviewThreadES(), adapters.ReviewThreadView(), func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.ReviewThread = rt

	// Wire the post-commit cross-aggregate reactions (spec §3.6): the workspace
	// delete reactor + its review-thread forget cascade, all joined to the shared
	// drain WaitGroup so graceful shutdown can quiesce them (Task 15). Both repos
	// must already be built — the cascade calls into c.ReviewThread.
	if err := c.wireCallbacks(ctx, adapters.CrowbarHome()); err != nil {
		return nil, fmt.Errorf("repositories: wire callbacks: %w", err)
	}
	return c, nil
}

// wireCallbacks registers the app-level cross-aggregate reactions on the singleton
// asynx instances (spec §3.6), mirroring quiver's container wireCallbacks. It
// creates and stores the shared drain WaitGroup + cancelable drain context every
// reactor it registers joins (decisions 9 + 11), reachable by the app layer via
// Drain() for the ordered graceful shutdown (Task 15). Today it registers the
// workspace delete reactor (Task 8) with the review-thread forget cascade and the
// bounded fs worktree delete; the two hub projections are already registered at
// construction (workspace hub in New above via RegisterHubProjection; reviewthread
// hub inside reviewthread.New), so re-registering them here would double-subscribe
// and double-broadcast — they are deliberately left where the live wiring puts them.
func (c *Container) wireCallbacks(
	ctx context.Context,
	crowbarHome string,
) error {
	c.drainWG = &sync.WaitGroup{}
	//nolint:gosec // G118: drainCancel is deliberately retained on the container and invoked later by the app layer's graceful shutdown via Drain().Cancel, not leaked.
	c.drainCtx, c.drainCancel = context.WithCancel(ctx)

	// The reactor lives under workspace/internal (unimportable from this out-of-tree
	// container), so it is registered through the repository's own seam, which hands
	// it the singleton axWorkspace + read model + id↔path map it holds privately.
	registrar, ok := c.Workspace.(workspace.DeleteReactorRegistrar)
	if !ok {
		return fmt.Errorf("workspace repository does not support delete-reactor registration")
	}
	if err := registrar.RegisterDeleteReactor(c.forgetReviewThreads, worktreeRemover(crowbarHome), c.drainWG); err != nil {
		return fmt.Errorf("delete reactor: %w", err)
	}
	return nil
}

// Drain exposes the shared reactor drain handle so the app layer's ordered graceful
// shutdown (Task 15) can quiesce every reactor wireCallbacks registered: Cancel
// closes the gate, then it waits on WG (bounded by the shutdown deadline) before the
// adapter closes the DBs (decisions 9 + 11).
func (c *Container) Drain() ReactorDrain {
	return ReactorDrain{Ctx: c.drainCtx, WG: c.drainWG, Cancel: c.drainCancel}
}

// forgetReviewThreads is the review-thread half of the workspace delete cascade
// (spec §3.6): every review thread anchored to the deleted workspace is Forgotten,
// and each Forget's synchronous OnForget drops that thread's read-model row. It is
// injected into the async delete reactor by wireCallbacks and runs post-commit, off
// the synchronous write path.
func (c *Container) forgetReviewThreads(
	ctx context.Context,
	wsID string,
) error {
	threads, err := c.ReviewThread.ListByWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("repositories: delete cascade: list review threads for %q: %w", wsID, err)
	}
	for _, t := range threads {
		if err := c.ReviewThread.DeleteThread(ctx, t.ID); err != nil {
			return fmt.Errorf("repositories: delete cascade: forget review thread %q: %w", t.ID, err)
		}
	}
	return nil
}

// enrichFrame builds the WS frame for ws: it attaches the two derived overlays
// that are NOT part of the event-sourced aggregate — the Working/inflight spinner
// (bracketed by BeginWork/EndWork, 00 §4) and the merge-eligibility overlay
// (CanMergeLocally/ParentBranch, resolved off the hot path from the row's
// repo-scoped siblings, spec §10) — and returns the wire DTO. It is the SINGLE
// enrichment both the hub projection (RegisterHubProjection) and the
// BeginWork/EndWork rebroadcasts converge on, so the emitted frame is identical
// regardless of trigger (spec §3.5 hub-frame enrichment).
func (c *Container) enrichFrame(
	ctx context.Context,
	ws domain.Workspace,
) dto.WorkspaceDTO {
	ws.Working = c.IsWorking(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	return dto.WorkspaceDTOFrom(ws, elig)
}

// broadcastWorkspace enriches ws and pushes it to the hub. It backs the
// BeginWork/EndWork rebroadcasts (which fire on the 202 ack, not on an event) and
// routes through the SAME enrichFrame as the hub projection so both agree.
func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	c.hub.BroadcastWorkspace(c.enrichFrame(ctx, ws))
}

// BeginWork marks the start of a background mutation on the workspace and
// immediately re-broadcasts its row with Working=true, so the client spinner
// starts the moment the 202 is written — not when the op's first event lands.
// Blank ids (a create that has not produced an entity yet) are ignored.
// Concurrent ops on the same workspace nest: the overlay stays true until the
// matching EndWork of the LAST one.
func (c *Container) BeginWork(
	ctx context.Context,
	wsID string,
) {
	if wsID == "" {
		return
	}
	c.mu.Lock()
	c.inflight[wsID]++
	c.mu.Unlock()
	c.rebroadcast(ctx, wsID)
}

// EndWork marks the end of a background mutation on the workspace and
// re-broadcasts its row so the final frame always carries Working=false (and
// whatever LastError the op recorded). Unbalanced calls never underflow, and a
// row deleted by the op itself just skips the re-broadcast (its tombstone
// already rode the event stream).
func (c *Container) EndWork(
	ctx context.Context,
	wsID string,
) {
	if wsID == "" {
		return
	}
	c.mu.Lock()
	if n := c.inflight[wsID]; n <= 1 {
		delete(c.inflight, wsID)
	} else {
		c.inflight[wsID] = n - 1
	}
	c.mu.Unlock()
	c.rebroadcast(ctx, wsID)
}

// IsWorking reports whether the workspace has a background mutation in flight.
func (c *Container) IsWorking(
	wsID string,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inflight[wsID] > 0
}

// rebroadcast pushes the workspace's current row to the hub so an overlay
// transition (Begin/EndWork) is visible without waiting for the op's next
// event. Best-effort: a missing row (already deleted) skips silently.
func (c *Container) rebroadcast(
	ctx context.Context,
	wsID string,
) {
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		return
	}
	c.broadcastWorkspace(ctx, ws)
}

// eligibilityFor resolves the merge-eligibility overlay (incl. the predicted
// merge-conflict flag) for ws by reading its siblings and delegating to the
// shared wsusecase.ResolveMergeEligibility — the SAME resolver the snapshot read
// path uses, so the live broadcast and the snapshot always agree. The sibling
// read is best effort — a failed List degrades to no eligibility rather than
// dropping the broadcast.
func (c *Container) eligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
) wsusecase.MergeEligibility {
	if ws.ParentID == "" {
		return wsusecase.MergeEligibility{}
	}
	siblings, err := c.ListWorkspacesInRepo(ctx, ws.ProjectID, ws.RepoID)
	if err != nil {
		return wsusecase.MergeEligibility{}
	}
	return wsusecase.ResolveMergeEligibility(ctx, ws, siblings, c.git)
}

// ListWorkspaces returns every workspace row with the derived Working overlay
// applied, so a snapshot-on-subscribe taken mid-mutation agrees with the live
// broadcast frames. It backs the Workspaces snapshot-on-subscribe.
func (c *Container) ListWorkspaces(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces: %w", err)
	}
	for i := range rows {
		rows[i].Working = c.IsWorking(rows[i].ID)
	}
	return rows, nil
}

// worktreeRemover builds the bounded fs delete the async delete reactor uses to
// rm -rf a deleted workspace's worktree, off the synchronous write path (spec
// §3.6, decision 9). It is GUARDED by the crowbar home: only a CROWBAR-MANAGED
// worktree (a path strictly under the home) is ever removed. An adopted home /
// main worktree's id↔path entry is the user's REAL checkout (repo.Path /
// project.Path, which live OUTSIDE the home) and must NEVER be deleted — the guard
// mirrors the synchronous project-delete removeWorktreeIfCrowbarManaged so both the
// delete reactor and the boot orphan-sweep converge without ever destroying a
// user's repository. A blank path, a path outside the home, or an already-gone dir
// is an idempotent no-op (os.RemoveAll returns nil for a missing path), so a crash
// re-driven cascade rm's to nothing.
func worktreeRemover(
	crowbarHome string,
) func(path string) error {
	return func(path string) error {
		if !managedWorktreePath(path, crowbarHome) {
			if path != "" {
				slog.Warn("repositories: refusing to rm worktree outside the crowbar home",
					"path", path, "home", crowbarHome)
			}
			return nil
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("repositories: remove worktree %q: %w", path, err)
		}
		return nil
	}
}

// managedWorktreePath reports whether path is a crowbar-managed worktree: a
// non-empty path strictly under the crowbar home. Adopted checkouts (repo.Path /
// project.Path) live outside the home and are excluded, so a delete/sweep never
// rm's the user's real repository (spec §3.9; the locked workspace-model law).
func managedWorktreePath(
	path string,
	crowbarHome string,
) bool {
	if path == "" || crowbarHome == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(crowbarHome, "/")+"/")
}

// ListWorkspacesInRepo returns every workspace row scoped to one project+repo,
// with the derived Working overlay applied, read from MY central store read
// model (state/store/workspace.db) filtered by project_id/repo_id via the
// workspace repo's ListInRepo — a single central-store read, not the
// whole-install per-entity scan the retired workspace_directory projection was
// built to avoid. It backs the repo-scoped snapshot-on-subscribe builders and
// the merge-eligibility overlay.
func (c *Container) ListWorkspacesInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	rows, err := c.Workspace.ListInRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces in repo: %w", err)
	}
	for i := range rows {
		rows[i].Working = c.IsWorking(rows[i].ID)
	}
	return rows, nil
}
