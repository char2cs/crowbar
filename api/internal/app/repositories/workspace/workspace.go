package workspace

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/reactors"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/reconcile"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
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
	// CreatedBranch: Crowbar created Branch for this workspace, so a teardown
	// may delete it (domain.Workspace.CreatedBranch).
	CreatedBranch bool
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
	// SetLock records the user's own lock decision, which outranks the
	// provider's protected flag from here on. A nil `locked` hands the question
	// back to the provider. `protected` is the provider's current answer, used
	// only to resolve the status when `locked` is nil.
	SetLock(
		ctx context.Context,
		id string,
		locked *bool,
		protected bool,
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
	// RenameBranch records a branch rename. The workspace does not move: its
	// directory is fixed at creation and never tracks the branch, so there is no
	// path to carry. Identity and lineage are untouched.
	RenameBranch(
		ctx context.Context,
		id string,
		branch string,
	) (domain.Workspace, error)
	// SetParentFromPR sets ParentID from an open PR's target branch without
	// recomputing ForkPointSha.
	SetParentFromPR(
		ctx context.Context,
		id string,
		parentID string,
	) (domain.Workspace, error)
	// SetProject re-points the workspace at the project that now owns its
	// repository, for a repo moved between projects. It moves no worktree.
	SetProject(
		ctx context.Context,
		id string,
		projectID string,
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
// composition root runs it once at boot; it re-drives the SAME Purger the delete
// reactor runs for every tombstone a crash left behind (spec §7-D). Kept OFF the
// main Workspace interface so a boot-recovery concern does not leak into the
// per-request repository surface; the concrete *workspace satisfies it.
type BootSweeper interface {
	Sweep(
		ctx context.Context,
	) error
}

// DeleteReactorRegistrar wires this aggregate's physical purge (Task 8) onto its
// singleton axWorkspace. The composition root injects the cross-aggregate forget
// cascade, the hardened worktree remover and the shared drain gate; this
// repository builds the one Purger from them and hands it to both the async
// delete reactor and the boot Sweep. Kept OFF the main Workspace interface (like
// BootSweeper); the concrete *workspace satisfies it.
type DeleteReactorRegistrar interface {
	RegisterDeleteReactor(
		forgetDependents func(ctx context.Context, wsID string) error,
		removeWorktree func(path string) error,
		gate *drain.Gate,
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
	reconciler ReconcileOnOpener
	// purger is the one physical purger, built by RegisterDeleteReactor and
	// shared by the delete reactor and the boot Sweep.
	purger *reactors.Purger
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

// New builds the singleton-backed Workspace repository over axWorkspace and the
// workspace read-model DB (state/store/workspace.db). es is the per-type event log axWorkspace wraps (state/events/workspace.db),
// retained so the read model can heal itself via whole-model lazy Replay on first
// access after a loss (§3.7). It registers the save-only store projection on
// axWorkspace via store.New; the hub projection is registered separately by
// repositories.Container (which owns the enrichment callback).
func New(
	ax asynx.Asynx[domain.Workspace],
	es asynxModels.Store,
	storeDB *gormdb.DB,
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
	readModel, err := store.New(storeDB, es, ax)
	if err != nil {
		return nil, fmt.Errorf("workspace: store: %w", err)
	}
	w := &workspace{ax: ax, readModel: readModel}
	for _, opt := range opts {
		opt(w)
	}
	return w, nil
}

// RegisterHubProjection registers the hub (WS fan-out) projection on repo's
// singleton axWorkspace: for every workspace event it builds the base frame from
// evt.Aggregate, runs enrich to attach the derived overlays the container owns
// (Working + merge eligibility), then broadcasts. It is generic over the frame
// type F so this package stays decoupled from the api-layer wire DTO the
// container supplies. Registered ONCE, by repositories.Container. A tombstone's
// frame is reported to the read model, and the delete reactor purges only once
// it is out: the frame is addressed through the owning chat the purge deletes.
func RegisterHubProjection[F any](
	repo Workspace,
	enrich func(ctx context.Context, ws domain.Workspace) F,
	broadcast func(frame F),
) error {
	w, ok := repo.(*workspace)
	if !ok {
		return fmt.Errorf("workspace: hub projection needs the concrete repository")
	}
	return store.RegisterHub(w.readModel, w.ax, enrich, broadcast)
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
//
// Cancellation is checked both before the timer starts and again after the select
// wakes: select breaks ties between two ready channels at random, so a context
// cancelled just before this call — with nothing left to make ctx.Done() the only
// ready case — could otherwise lose that toss to a timer.C that also fires by the
// time select runs, letting one more send attempt slip out after the caller gave up.
func occBackoff(ctx context.Context, attempt int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
		if err := ctx.Err(); err != nil {
			return err
		}
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
		CreatedBranch: in.CreatedBranch,
		Now:           now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: create: %w", err)
	}
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
	cmd := commands.SyncProviderState{
		ID:             in.ID,
		Protected:      in.Protected,
		HasPR:          in.HasPR,
		PRStatus:       in.PRStatus,
		PRUrl:          in.PRUrl,
		PRTitle:        in.PRTitle,
		PRTargetBranch: in.PRTargetBranch,
		Now:            now,
	}
	// A poll that changes nothing must not write. The sweep visits every workspace
	// every 5 minutes, and each visit appended an event AND a snapshot whether or
	// not the provider had moved: in a real home, 43,970 of 44,068 provider_synced
	// events carried an empty patch set, and every one of them bumped the
	// aggregate version that OCC and the delete cascade race against.
	//
	// EmitEvent is pure, so applying it here answers exactly the question the write
	// would have asked. Read through ax directly, NOT w.Get: Get triggers the lazy
	// reconcile (OnOpen), whose own job is to re-derive provider reality and sync —
	// so reading that way would have this call schedule the work that calls it.
	if cur, getErr := w.ax.Get(ctx, in.ID); getErr == nil && cmd.EmitEvent(&cur) == cur {
		return cur, nil
	}
	evt, err := w.sendWithOCC(ctx, cmd)
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

func (w *workspace) SetLock(
	ctx context.Context,
	id string,
	locked *bool,
	protected bool,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SetLock{ID: id, Locked: locked, Protected: protected})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set lock: %w", err)
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
		ID:              id,
		NewForkParentID: parentID,
		ForkPointSha:    forkPointSha,
		Now:             now,
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

func (w *workspace) RenameBranch(
	ctx context.Context,
	id string,
	branch string,
) (domain.Workspace, error) {
	// WorktreePath is deliberately NOT touched. A workspace's directory is fixed
	// at creation, so a rename moves nothing, and the purge removes exactly the
	// tombstone's WorktreePath.
	evt, err := w.sendWithOCC(ctx, commands.RenameBranch{ID: id, Branch: branch})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: rename branch: %w", err)
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

func (w *workspace) SetProject(
	ctx context.Context,
	id string,
	projectID string,
) (domain.Workspace, error) {
	evt, err := w.sendWithOCC(ctx, commands.SetProject{ID: id, ProjectID: projectID})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set project: %w", err)
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
//
// It returns only once every projection has handled the tombstone — SendWait,
// not Send. The tombstone's frame is addressed through the workspace's owning
// chat, and a caller routinely purges that chat next (a chat delete, the delete
// reactor); a purge that beat the hub projection dropped the frame and left the
// client a ghost row. Committing faster (synchronous=NORMAL) made that race win
// often enough to see.
func (w *workspace) Delete(
	ctx context.Context,
	id string,
) error {
	_, err := occSend(ctx, w.ax.SendWait, commands.Delete{ID: id})
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
		if inRepo(ws, projectID, repoID) {
			rows = append(rows, ws)
		}
	}
	return rows, nil
}

// inRepo reports whether ws is one of repoID's rows. A repo belongs to exactly
// one project, and the REPO row is the one owner of that assignment: a repo's
// rows are its rows whatever their own (denormalised) ProjectID says, so a repo
// move that relocated only some of them before failing never hides the rest
// (spec §3 P0-3). A repo-less row (a project home) is scoped by its project.
func inRepo(
	ws domain.Workspace,
	projectID string,
	repoID string,
) bool {
	if repoID != "" {
		return ws.RepoID == repoID
	}
	return ws.RepoID == "" && ws.ProjectID == projectID
}

// Sweep runs the boot orphan-sweep over this repository's RAW read model (spec
// §3.8): it reads state/store/workspace.db DIRECTLY — never the Replay-wrapped
// per-request List — so boot pays no replay and an empty model reaps nothing,
// and re-drives the one Purger for every residual "deleted" row, from that
// tombstone's own WorktreePath. Best-effort per row: recovery work never fails
// boot. It refuses to run before RegisterDeleteReactor has built the Purger.
func (w *workspace) Sweep(
	ctx context.Context,
) error {
	if w.purger == nil {
		return fmt.Errorf("workspace: sweep: no purger registered")
	}
	reconcile.NewSweeper(reconcile.SweepListFunc(w.readModel.List), w.purger.Purge).Sweep(ctx)
	return nil
}

// RegisterDeleteReactor builds the one physical Purger from the injected
// cross-aggregate forget cascade and hardened worktree remover, keeps it for the
// boot Sweep, and subscribes the async delete reactor to run it over the
// persisted tombstone (spec §3.6/§3.8, §7-D).
func (w *workspace) RegisterDeleteReactor(
	forgetDependents func(ctx context.Context, wsID string) error,
	removeWorktree func(path string) error,
	gate *drain.Gate,
) error {
	w.purger = reactors.NewPurger(w.ax, w.readModel.Drop, forgetDependents, removeWorktree)
	return reactors.RegisterDeleteReactor(w.ax, w.readModel, w.purger, gate)
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

// homeWorkspaceNamespace is this package's own RFC 4122 namespace for every
// name-derived id it mints — currently just a project's home workspace, but
// kept as its own namespace (rather than reusing uuid.NameSpaceURL or the
// like) so a deterministic id minted here can never collide with one some
// unrelated part of the system derives the same way from an unrelated name.
var homeWorkspaceNamespace = uuid.MustParse("f9a1b2c3-2026-4a1a-8b1c-c70de5e8f001")

// homeWorkspaceID derives a project's home workspace id deterministically
// from its project id, in place of minting a fresh random one on every call.
//
// This is the actual fix for a live bug that used to duplicate a project's
// home workspace under concurrent requests: two callers racing "create the
// home for project P," with nothing else serializing them, both saw no home
// workspace yet and both called Create. A RANDOM id gives asynx's own
// per-aggregate concurrency control nothing to enforce — the two calls were
// never contending for the same aggregate at all, so both simply succeeded,
// leaving the project with two home workspaces and every later caller
// racing to guess which one is "real."
//
// A deterministic id fixes that at the root, not around it: two concurrent
// creates for the SAME project now target the IDENTICAL aggregate id, which
// asynx serializes through that one aggregate's own command queue exactly
// like every other write in this system (see occSend's own doc). The first
// commits; CreateWorkspace's own Validate ("if current != nil, refuse")
// rejects the second outright, deterministically, with no timing window at
// all — the identical guarantee every other aggregate in this codebase
// already relies on, just extended to a name-derived id instead of a
// server-randomised one. No new locking primitive, no process-local mutex:
// a second daemon instance, or a retried request years apart, gets the
// exact same outcome.
func homeWorkspaceID(projectID string) string {
	return uuid.NewSHA1(homeWorkspaceNamespace, []byte(projectID)).String()
}

// RepoHomeID derives a repo's home (IsDefault) workspace id deterministically
// from the repo id, for the reason homeWorkspaceID gives: two concurrent adopts
// of the same repo's main folder target the SAME aggregate, so the second is
// refused by CreateWorkspace's own Validate instead of leaving the repo with two
// default workspaces (invariant D2). Namespaced apart from the project home's.
func RepoHomeID(repoID string) string {
	return uuid.NewSHA1(homeWorkspaceNamespace, []byte("repo-home:"+repoID)).String()
}

// CreateHome provisions the home workspace for a project, used for lazy
// provisioning when GetHomeForProject returns ErrNotFound.
//
// Idempotent under real concurrency, not merely safe: a caller that loses
// the race homeWorkspaceID's own doc describes does not get an error back at
// all — it reads the winner's own committed workspace directly from the
// event store (never the read model, which may still be catching up to that
// commit) and returns it exactly as if it had won itself. Reading it there
// rather than surfacing apperr.ErrConflict for a caller to recover from is
// safe done HERE, unlike inside the shared Create/Validate this calls
// through, because every argument on THIS call is fixed by construction (a
// non-empty id, WorkspaceKindHome needing no RepoID): the ONLY way
// CreateWorkspace's Validate can refuse it is "current != nil," never one of
// its other validation branches, so an ErrValidation reaching here can only
// ever mean one thing — mirrors node.EventStore.CreateIdempotent's own,
// identically-scoped reasoning.
func (w *workspace) CreateHome(ctx context.Context, projectID, worktreePath string, now time.Time) (domain.Workspace, error) {
	id := homeWorkspaceID(projectID)
	ws, err := w.Create(ctx, CreateInput{
		ID:           id,
		ProjectID:    projectID,
		WorktreePath: worktreePath,
		Kind:         domain.WorkspaceKindHome,
	}, now)
	if err == nil {
		return ws, nil
	}
	if !errors.Is(err, asynxModels.ErrValidation) {
		return domain.Workspace{}, fmt.Errorf("create home workspace: %w", err)
	}
	won, getErr := w.ax.Get(ctx, id)
	if getErr != nil {
		// The winner's commit isn't visible yet even at the event-store layer
		// (Get, not the read model) — genuinely unexpected for a same-process
		// serialized aggregate, so surface the ORIGINAL refusal rather than a
		// getErr that names no cause a caller could act on.
		return domain.Workspace{}, fmt.Errorf("create home workspace: %w", err)
	}
	return won, nil
}
