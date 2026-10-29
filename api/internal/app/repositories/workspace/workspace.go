package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/reactors"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/reconcile"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// workspace is the singleton-backed workspace aggregate repository. One
// axWorkspace routes every workspace id to a shard by hash; there is no per-entity
// Registry, no writeMu (asynx serializes each aggregate's commands), and no
// location index (the store read model carries
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

// New builds the Workspace repository over axWorkspace, which must have been
// built with Subscriber(rm). es is the log axWorkspace wraps, kept so the read
// model can heal itself by lazy Replay after a loss (§3.7).
func New(
	ax asynx.Asynx[domain.Workspace],
	es asynxModels.Store,
	rm *ReadModel,
	opts ...Option,
) (Workspace, error) {
	if ax == nil {
		return nil, fmt.Errorf("workspace: nil asynx")
	}
	if es == nil {
		return nil, fmt.Errorf("workspace: nil event store")
	}
	if rm == nil {
		return nil, fmt.Errorf("workspace: nil read model")
	}
	w := &workspace{ax: ax, readModel: store.New(rm.st, es, ax)}
	for _, opt := range opts {
		opt(w)
	}
	return w, nil
}

func (w *workspace) Create(
	ctx context.Context,
	in CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	// SendWait: the row is readable (GetHomeForProject, ListInRepo) when Create
	// returns, so whatever the caller announces next can be looked up at once.
	evt, err := w.ax.SendWait(ctx, commands.CreateWorkspace{
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
		Provisioning:  in.Provisioning,
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
	evt, err := w.ax.Send(ctx, commands.SyncWorkingTreeState{
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
	// aggregate version.
	//
	// EmitEvent is pure, so applying it here answers exactly the question the write
	// would have asked. Read through ax directly, NOT w.Get: Get triggers the lazy
	// reconcile (OnOpen), whose own job is to re-derive provider reality and sync —
	// so reading that way would have this call schedule the work that calls it.
	if cur, getErr := w.ax.Get(ctx, in.ID); getErr == nil && cmd.EmitEvent(&cur) == cur {
		return cur, nil
	}
	evt, err := w.ax.Send(ctx, cmd)
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
	evt, err := w.ax.Send(ctx, commands.SetMergeStrategy{ID: id, Strategy: strategy})
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
	evt, err := w.ax.Send(ctx, commands.SetLock{ID: id, Locked: locked, Protected: protected})
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
	evt, err := w.ax.Send(ctx, commands.TouchActivity{ID: id, Now: now})
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
	evt, err := w.ax.Send(ctx, commands.Reparent{
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
	evt, err := w.ax.Send(ctx, commands.ResolveConflicts{
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
	evt, err := w.ax.Send(ctx, commands.UpdateForkPoint{ID: id, ForkPointSha: forkPointSha})
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
	evt, err := w.ax.Send(ctx, commands.ProvisionInPlace{
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
	evt, err := w.ax.Send(ctx, commands.ClearBranch{ID: id})
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
	evt, err := w.ax.Send(ctx, commands.RenameBranch{ID: id, Branch: branch})
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
	evt, err := w.ax.Send(ctx, commands.SetParentFromPR{ID: id, ParentID: parentID})
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
	evt, err := w.ax.Send(ctx, commands.SetLastError{ID: id, Message: message})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set last error: %w", err)
	}
	return evt.Aggregate, nil
}

// Delete tombstones the workspace via the pure Delete command: it
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
	_, err := w.ax.SendWait(ctx, commands.Delete{ID: id})
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

// BackfillProvisioning records Provisioning on every row of the RAW read model
// that lacks it. Best-effort per row: a row whose history cannot be replayed is
// logged and left for the next boot, and never keeps the daemon from starting.
func (w *workspace) BackfillProvisioning(
	ctx context.Context,
) {
	rows, err := w.readModel.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "workspace: backfill provisioning: list", "err", err)
		return
	}
	for _, ws := range rows {
		if ws.Provisioning != "" {
			continue
		}
		if _, err := w.ax.Send(ctx, commands.BackfillProvisioning{ID: ws.ID}); err != nil &&
			!errors.Is(err, asynxModels.ErrValidation) {
			slog.ErrorContext(ctx, "workspace: backfill provisioning (continuing)",
				"workspace_id", ws.ID, "err", err)
		}
	}
}

// Sweep runs the boot orphan-sweep over this repository's RAW read model (spec
// §3.8): it reads state/store/workspace.db DIRECTLY — never the Replay-wrapped
// per-request List — so boot pays no replay and an empty model reaps nothing,
// and re-drives the one Purger for every residual "deleted" row, from that
// tombstone's own WorktreePath. Best-effort per row: recovery work never fails
// boot. It refuses to run before RegisterDeleteReactor has built the Purger.
func (w *workspace) Sweep(
	ctx context.Context,
	onPurged func(ctx context.Context, wsID string),
) error {
	if w.purger == nil {
		return fmt.Errorf("workspace: sweep: no purger registered")
	}
	reconcile.NewSweeper(reconcile.SweepListFunc(w.readModel.List), w.purger.Purge).Sweep(ctx, onPurged)
	return nil
}

// RegisterDeleteReactor builds the one physical Purger from the injected
// cross-aggregate forget cascade and hardened worktree remover, keeps it for the
// boot Sweep, and subscribes the async delete reactor to run it over the
// persisted tombstone (spec §3.6/§3.8, §7-D).
func (w *workspace) RegisterDeleteReactor(
	root context.Context,
	forgetDependents func(ctx context.Context, wsID string) error,
	removeWorktree func(ctx context.Context, tomb domain.Workspace) error,
	gate *drain.Gate,
	onGiveUp func(ctx context.Context, wsID string, tomb *domain.Workspace, err error),
) error {
	w.purger = reactors.NewPurger(w.ax, w.readModel.Drop, forgetDependents, removeWorktree)
	return reactors.RegisterDeleteReactor(root, w.ax, w.readModel, w.purger, gate, onGiveUp)
}

// GetHomeForProject scans all workspaces for the project and returns the one
// whose Kind is WorkspaceKindHome. Returns problem.ErrNotFound when absent.
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
	return domain.Workspace{}, fmt.Errorf("get home for project %q: %w", projectID, problem.ErrNotFound)
}
