package workspace

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/reactors"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/reconcile"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// maxOCCAttempts bounds optimistic-concurrency retries on ErrPipelineFailed
// (decision 10): with writeMu deleted, concurrent Sends to one aggregate id can
// version-collide, so the losers retry — Send re-reads the current version each
// attempt (the shard's pre-assigned version is ignored by the event store), so a
// retry converges. The budget is sized for a burst of concurrent same-aggregate
// commands: each needs its own version slot, so the tail committer may lose to
// several winners before landing. occBackoff spreads the retries (full jitter) so
// they converge without exhausting the budget on lockstep re-collisions.
// ErrValidation is NEVER retried; ErrQueueFull is surfaced.
const maxOCCAttempts = 16

// CreateInput carries the fields needed to create a workspace.
type CreateInput struct {
	ID            string
	RepoID        string
	ProjectID     string
	Branch        string
	WorktreePath  string
	ForkPointSha  string
	ParentID      string
	Protected     bool
	MergeStrategy gitdomain.MergeStrategy
	IsDefault     bool
	Kind          domain.WorkspaceKind
	HeldByPath    string
}

// SyncInput carries a recomputed working-tree summary.
type SyncInput struct {
	ID           string
	Added        int
	Deleted      int
	HasConflicts bool
	HasCommits   bool
}

// ProviderInput carries a provider poll result (08 §5).
type ProviderInput struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}

// Workspace is the workspace aggregate repository.
type Workspace interface {
	Create(
		ctx context.Context,
		in CreateInput,
		now time.Time,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in SyncInput,
		now time.Time,
	) (domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SyncProviderState(
		ctx context.Context,
		in ProviderInput,
		now time.Time,
	) (domain.Workspace, error)
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	TouchActivity(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	Reparent(
		ctx context.Context,
		id string,
		parentID string,
		forkPointSha string,
		now time.Time,
	) (domain.Workspace, error)
	ResolveConflicts(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	UpdateForkPoint(
		ctx context.Context,
		id string,
		forkPointSha string,
	) (domain.Workspace, error)
	// ProvisionInPlace attaches a worktree to a placeholder row (spec §3.3): it
	// records worktreePath + forkPointSha and clears HeldByPath, keeping Status.
	ProvisionInPlace(
		ctx context.Context,
		id string,
		worktreePath string,
		forkPointSha string,
	) (domain.Workspace, error)
	// ClearBranch blanks an existing aggregate's Branch to "" (spec §4/B6),
	// leaving every other field untouched. Used by the Detach-holder op when the
	// holder is the repo home.
	ClearBranch(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	// SetParentFromPR sets ParentID from an open PR's target branch without
	// recomputing ForkPointSha.
	SetParentFromPR(
		ctx context.Context,
		id string,
		parentID string,
	) (domain.Workspace, error)
	// SetLastError records the message from a failed background operation on the
	// workspace; the failure surfaces on the entity, never a separate WS frame
	// (00 §4). The next successful mutating command clears it.
	SetLastError(
		ctx context.Context,
		id string,
		message string,
	) (domain.Workspace, error)
	// Delete tombstones the workspace: it fires the pure Delete command (folds
	// Status=deleted) via Send. The physical teardown (cascade Forgets + rm -rf +
	// axWorkspace.Forget) runs off the write path in the async delete reactor
	// (spec §3.6/§3.8, Task 8).
	Delete(
		ctx context.Context,
		id string,
	) error
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	// ListInRepo returns every workspace row scoped to one project+repo. It reads
	// the same central durable read model as List (state/store/workspace.db) and
	// filters by projectID+repoID, so it has identical read-after-write
	// consistency to List — the merge-eligibility broadcast overlay uses it so a
	// broadcast of a parented workspace resolves its siblings without materializing
	// the whole install.
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
	// GetHomeForProject returns the home workspace for the given project.
	// Returns apperr.ErrNotFound if no home workspace exists yet.
	GetHomeForProject(
		ctx context.Context,
		projectID string,
	) (domain.Workspace, error)
	// CreateHome provisions the home workspace for a project. Callers use this
	// for lazy provisioning when GetHomeForProject returns ErrNotFound.
	CreateHome(
		ctx context.Context,
		projectID string,
		worktreePath string,
		now time.Time,
	) (domain.Workspace, error)
}

// ReconcileOnOpener triggers a lazy, deduplicated, one-shot background reconcile
// for a single workspace id off the per-id read path (spec §3.8). Get/detail
// calls OnOpen; List never does. It is injected via WithReconciler so the
// git+provider re-derivation stays owned by a higher layer and this repository
// keeps no git/provider dependency.
type ReconcileOnOpener interface {
	OnOpen(
		ctx context.Context,
		wsID string,
	)
}

// BootSweeper is the boot orphan-sweep seam (spec §3.8). The app-layer
// composition root reaches this repository's RAW read model — the direct
// state/store/workspace.db read that never triggers the lazy Replay of the
// per-request List (§3.7) — through this narrow interface, injecting the
// idempotent purge it owns (the app layer holds the git/fs/asynx concretes; this
// repository must not). It is kept OFF the main Workspace interface so a
// boot-recovery concern does not leak into the per-request repository surface and
// existing Workspace fakes stay untouched; the concrete *workspace satisfies it.
type BootSweeper interface {
	Sweep(
		ctx context.Context,
		purge func(ctx context.Context, wsID string) error,
	)
}

// DeleteReactorRegistrar wires this aggregate's async delete reactor (Task 8) onto
// its singleton axWorkspace. It is the app-level seam (Task 14 wireCallbacks) for
// the post-commit cross-aggregate purge: the app-level composition root injects the
// review-thread forget cascade, the bounded fs delete, and the shared drain
// WaitGroup, while this repository keeps its ax / read model / id↔path handles
// private — the reactor gates on the read model's persisted "deleted" tombstone,
// resolves the worktree path via the id↔path map, cascades the forget, rm -rf's the
// worktree, and Forgets the aggregate (spec §3.6/§3.8). The reactor lives under
// workspace/internal, so an out-of-tree caller (repositories.Container) cannot reach
// it directly; this method is the seam. Kept OFF the main Workspace interface (like
// BootSweeper) so cross-aggregate wiring never leaks into the per-request surface
// and existing Workspace fakes stay untouched; the concrete *workspace satisfies it.
type DeleteReactorRegistrar interface {
	RegisterDeleteReactor(
		reviewThreadForget func(ctx context.Context, wsID string) error,
		rmWorktree func(path string) error,
		drainWG *sync.WaitGroup,
	) error
}

// workspace is the singleton-backed workspace aggregate repository. One
// axWorkspace routes every workspace id to a shard by hash; there is no per-entity
// Registry, no writeMu (per-aggregate safety is shard routing + (id,version)
// uniqueness + OCC retry), and no location index (the store read model carries
// project_id/repo_id and doubles as the location index — §3.7).
type workspace struct {
	ax         asynx.Asynx[domain.Workspace]
	readModel  store.Store
	pathsStore wspaths.WorkspacePaths
	reconciler ReconcileOnOpener
}

// Option configures the workspace repository at construction.
type Option func(*workspace)

// WithReconciler wires the reconcile-on-open trigger consulted by Get (spec
// §3.8): the first per-id open dispatches a deduped background reconcile. Absent
// it, Get is a pure read-model fold with no reconcile.
func WithReconciler(
	r ReconcileOnOpener,
) Option {
	return func(w *workspace) {
		w.reconciler = r
	}
}

// New builds the singleton-backed Workspace repository over axWorkspace, the
// workspace read-model DB (state/store/workspace.db), and the view.db id↔path
// index. es is the per-type event log axWorkspace wraps (state/events/workspace.db),
// retained so the read model can heal itself via whole-model lazy Replay on first
// access after a loss (§3.7). It registers the save-only store projection on
// axWorkspace via store.New; the hub projection is registered separately by
// repositories.Container (which owns the enrichment callback).
func New(
	ax asynx.Asynx[domain.Workspace],
	es asynxModels.Store,
	storeDB *gormdb.DB,
	pathsStore wspaths.WorkspacePaths,
	opts ...Option,
) (Workspace, error) {
	if ax == nil {
		return nil, fmt.Errorf("workspace: nil asynx")
	}
	if es == nil {
		return nil, fmt.Errorf("workspace: nil event store")
	}
	if storeDB == nil {
		return nil, fmt.Errorf("workspace: nil store db")
	}
	if pathsStore == nil {
		return nil, fmt.Errorf("workspace: nil paths store")
	}
	readModel, err := store.New(storeDB, es, ax)
	if err != nil {
		return nil, fmt.Errorf("workspace: store: %w", err)
	}
	w := &workspace{ax: ax, readModel: readModel, pathsStore: pathsStore}
	for _, opt := range opts {
		opt(w)
	}
	return w, nil
}

// RegisterHubProjection registers the hub (WS fan-out) projection on the singleton
// axWorkspace: for every workspace event it builds the base frame from
// evt.Aggregate, runs enrich to attach the derived overlays the container owns
// (Working + merge eligibility), then broadcasts. It is generic over the frame
// type F so this package stays decoupled from the api-layer wire DTO the container
// supplies. Registered ONCE, by repositories.Container (which owns enrich +
// broadcast, and routes BeginWork/EndWork through the SAME pair); the save-only
// store projection is registered inside New. The projections subpackage lives
// under workspace/internal, so this forwarder is the seam the container reaches it
// through (spec §3.5 hub-frame enrichment, decision 5).
func RegisterHubProjection[F any](
	ax asynx.Asynx[domain.Workspace],
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) error {
	return projections.RegisterHub(ax, enrich, broadcast)
}

// sendFunc issues one command attempt against the aggregate.
type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error)

// occSend runs send with OCC retry and the terminal error disposition contract
// (spec §3.5, decision 10):
//
//   - success                → returned immediately.
//   - ErrValidation          → surfaced immediately, NEVER retried (→ 422).
//   - ErrQueueFull           → translated to apperr.ErrUnavailable (→ 503),
//     NEVER retried: a full shard queue is backpressure, not a version race.
//   - ErrPipelineFailed      → retried up to maxOCCAttempts; still failing after
//     the retries is an unrecoverable optimistic-concurrency collision, surfaced
//     as ErrPipelineFailed (→ 409).
//   - any other error        → surfaced as-is.
//
// All classification is via errors.Is, never string compare.
func occSend(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error) {
	var lastErr error
	for attempt := range maxOCCAttempts {
		evt, err := send(ctx, cmd)
		if err == nil {
			return evt, nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return asynxModels.Event[domain.Workspace]{}, err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("workspace: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
			// Back off before retrying so version losers do not re-read and re-collide
			// in lockstep: without a jittered pause, heavy same-aggregate contention can
			// exhaust maxOCCAttempts even though a serialised commit order exists (OCC
			// livelock). Full-jitter exponential backoff desynchronises contenders so
			// they converge within the budget. No wait after the final attempt, and a
			// cancelled context aborts the wait. The happy path never reaches here (the
			// first send commits), so this adds zero latency without contention.
			if attempt < maxOCCAttempts-1 {
				if werr := occBackoff(ctx, attempt); werr != nil {
					return asynxModels.Event[domain.Workspace]{}, werr
				}
			}
		default:
			return asynxModels.Event[domain.Workspace]{}, err
		}
	}
	return asynxModels.Event[domain.Workspace]{}, lastErr
}

// OCC retry backoff is capped full-jitter exponential: retry attempt k (0-based)
// waits a random duration in [0, min(occBackoffBase·2^k, occBackoffCap)). The base
// is sub-millisecond so early retries stay fast; the cap keeps the deepest retries
// from ballooning latency while still spreading contenders across a wide window.
const (
	occBackoffBase = 200 * time.Microsecond
	occBackoffCap  = 2 * time.Millisecond
)

// occBackoff sleeps a capped full-jitter exponential backoff for the 0-based retry
// attempt, returning ctx.Err() early if the context is cancelled first. math/rand/v2
// is goroutine-safe, so concurrent contenders draw independent jitter.
func occBackoff(ctx context.Context, attempt int) error {
	window := occBackoffBase << attempt // occBackoffBase · 2^attempt
	if window > occBackoffCap || window <= 0 {
		window = occBackoffCap
	}
	timer := time.NewTimer(time.Duration(rand.Int64N(int64(window))))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// sendWithOCC dispatches cmd to the singleton axWorkspace with OCC retry.
func (w *workspace) sendWithOCC(
	ctx context.Context,
	cmd asynxModels.Command[domain.Workspace],
) (asynxModels.Event[domain.Workspace], error) {
	return occSend(ctx, w.ax.Send, cmd)
}

func (w *workspace) Create(
	ctx context.Context,
	in CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	// Record the id→path row before the aggregate exists (§3.9 write-point (a)):
	// it is the rename-resilience map, keyed by the workspace UUID. Roll it back
	// unless the create commits, so a failed create leaves no orphan path row.
	if err := w.pathsStore.Put(ctx, in.ID, in.WorktreePath); err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: paths: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = w.pathsStore.Delete(ctx, in.ID)
		}
	}()
	evt, err := w.sendWithOCC(ctx, commands.CreateWorkspace{
		ID:            in.ID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  in.WorktreePath,
		ForkPointSha:  in.ForkPointSha,
		ParentID:      in.ParentID,
		Protected:     in.Protected,
		IsDefault:     in.IsDefault,
		MergeStrategy: in.MergeStrategy,
		Kind:          in.Kind,
		HeldByPath:    in.HeldByPath,
		Now:           now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
	committed = true
	return evt.Aggregate, nil
}

func (w *workspace) SyncWorkingTreeState(
	ctx context.Context,
	in SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SyncWorkingTreeState{
		ID:           in.ID,
		Added:        in.Added,
		Deleted:      in.Deleted,
		HasConflicts: in.HasConflicts,
		HasCommits:   in.HasCommits,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	// Per-id reads fold the aggregate directly from the event log (§3.7), so Get
	// is always current and needs no read-model rebuild. asynx returns ErrNotFound
	// for an unknown id, which handlers map to 404.
	got, err := w.ax.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	// A per-id open triggers a lazy, deduplicated, background reconcile off this
	// read path (spec §3.8): Get returns immediately from the folded aggregate
	// while the reconcile task re-derives git+provider reality and SendWaits a
	// pure sync command. List never triggers this.
	if w.reconciler != nil {
		w.reconciler.OnOpen(ctx, id)
	}
	return got, nil
}

func (w *workspace) SyncProviderState(
	ctx context.Context,
	in ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SyncProviderState{
		ID:             in.ID,
		Protected:      in.Protected,
		HasPR:          in.HasPR,
		PRStatus:       in.PRStatus,
		PRUrl:          in.PRUrl,
		PRTitle:        in.PRTitle,
		PRTargetBranch: in.PRTargetBranch,
		Now:            now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync provider: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SetMergeStrategy{ID: id, Strategy: strategy})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set merge strategy: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) TouchActivity(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.TouchActivity{ID: id, Now: now})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: touch activity: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Reparent(
	ctx context.Context,
	id string,
	parentID string,
	forkPointSha string,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.Reparent{
		ID:           id,
		ParentID:     parentID,
		ForkPointSha: forkPointSha,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: reparent: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ResolveConflicts(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.ResolveConflicts{
		ID:  id,
		Now: now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: resolve conflicts: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.UpdateForkPoint{ID: id, ForkPointSha: forkPointSha})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: update fork point: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ProvisionInPlace(
	ctx context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.ProvisionInPlace{
		ID:           id,
		WorktreePath: worktreePath,
		ForkPointSha: forkPointSha,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: provision in place: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ClearBranch(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.ClearBranch{ID: id})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: clear branch: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetParentFromPR(
	ctx context.Context,
	id string,
	parentID string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SetParentFromPR{ID: id, ParentID: parentID})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set parent from pr: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetLastError(
	ctx context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SetLastError{ID: id, Message: message})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set last error: %w", err)
	}
	return evt.Aggregate, nil
}

// Delete tombstones the workspace via the pure Delete command (Send + OCC): it
// folds Status=deleted and does NO io. The store projection persists the deleted
// row (so the boot orphan-sweep still finds it) and the async delete reactor
// (topic "workspace.deleted.*", Task 8) performs the physical teardown off the
// write path — closing the old synchronous forget→rm crash gap (spec §3.6/§3.8).
func (w *workspace) Delete(
	ctx context.Context,
	id string,
) error {
	_, err := w.sendWithOCC(ctx, commands.Delete{ID: id})
	if err != nil {
		return fmt.Errorf("workspace: delete: %w", err)
	}
	return nil
}

// List returns every workspace row from the durable read model
// (state/store/workspace.db), which doubles as the location index (§3.7). It reads
// the projection directly and MUST NOT trigger any per-workspace reconcile
// (git/provider re-derivation, §3.8) — but it DOES heal a lost read model via
// whole-model lazy Replay when the model is empty while the event log is non-empty
// (§3.7, decision 7), hence ListOrRebuild rather than the raw List (which is
// reserved for the boot orphan-sweep, so startup pays no replay).
func (w *workspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := w.readModel.ListOrRebuild(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return rows, nil
}

// ListInRepo returns every workspace row scoped to projectID+repoID. It reads
// the durable central read model via List (state/store/workspace.db) and filters
// in memory — a single central-store read, not a per-install scan — mirroring
// GetHomeForProject. Each read-model row carries project_id/repo_id off the
// folded aggregate (spec §3.7), so the central store natively serves the
// repo-scoped query with no separate location/directory table.
func (w *workspace) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	all, err := w.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list in repo: %w", err)
	}
	rows := make([]domain.Workspace, 0, len(all))
	for _, ws := range all {
		if ws.ProjectID == projectID && ws.RepoID == repoID {
			rows = append(rows, ws)
		}
	}
	return rows, nil
}

// Sweep runs the boot orphan-sweep over this repository's RAW read model (spec
// §3.8). It reads state/store/workspace.db DIRECTLY via w.readModel.List — NOT
// the Replay-wrapped per-request List path — so boot pays no replay and an empty
// model reaps nothing (§3.7/§3.8). For every residual Status="deleted" row it
// re-drives the caller-supplied idempotent purge (cascade Forget + rm -rf worktree
// + axWorkspace.Forget). Best-effort throughout: recovery work never fails boot.
func (w *workspace) Sweep(
	ctx context.Context,
	purge func(ctx context.Context, wsID string) error,
) {
	reconcile.NewSweeper(reconcile.SweepListFunc(w.readModel.List), purge).Sweep(ctx)
}

// RegisterDeleteReactor subscribes the async delete reactor to this repo's
// singleton axWorkspace, handing it this repo's own private handles — axWorkspace,
// the durable read model (the ordering-gate StoreReader; w.readModel satisfies
// reactors.StoreReader via its Get), and the id↔path map — and the app-injected
// cross-aggregate deps: the review-thread forget cascade, the bounded fs delete,
// and the shared drain WaitGroup every reactor goroutine joins (spec §3.6/§3.8,
// Task 8/14). It is the seam repositories.Container reaches the internal reactor
// through (the reactors package is under workspace/internal, unimportable from the
// out-of-tree container).
func (w *workspace) RegisterDeleteReactor(
	reviewThreadForget func(ctx context.Context, wsID string) error,
	rmWorktree func(path string) error,
	drainWG *sync.WaitGroup,
) error {
	return reactors.RegisterDeleteReactor(
		w.ax,
		w.readModel,
		w.pathsStore,
		reviewThreadForget,
		rmWorktree,
		drainWG,
	)
}

// GetHomeForProject scans all workspaces for the project and returns the one
// whose Kind is WorkspaceKindHome. Returns apperr.ErrNotFound when absent.
func (w *workspace) GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error) {
	all, err := w.List(ctx)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("get home for project: list: %w", err)
	}
	for _, ws := range all {
		if ws.ProjectID == projectID && ws.Kind == domain.WorkspaceKindHome {
			return ws, nil
		}
	}
	return domain.Workspace{}, fmt.Errorf("get home for project %q: %w", projectID, apperr.ErrNotFound)
}

// CreateHome provisions the home workspace for a project, used for lazy
// provisioning when GetHomeForProject returns ErrNotFound.
func (w *workspace) CreateHome(ctx context.Context, projectID, worktreePath string, now time.Time) (domain.Workspace, error) {
	ws, err := w.Create(ctx, CreateInput{
		ID:           uuid.NewString(),
		ProjectID:    projectID,
		WorktreePath: worktreePath,
		Kind:         domain.WorkspaceKindHome,
	}, now)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create home workspace: %w", err)
	}
	return ws, nil
}
