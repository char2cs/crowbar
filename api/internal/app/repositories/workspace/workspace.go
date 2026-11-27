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

type workspace struct {
	ax         asynx.Asynx[domain.Workspace]
	readModel  store.Store
	reconciler ReconcileOnOpener
	purger     *reactors.Purger
}

type Option func(*workspace)

func WithReconciler(
	r ReconcileOnOpener,
) Option {
	return func(w *workspace) {
		w.reconciler = r
	}
}

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
	evt, err := w.ax.SendWait(ctx, syncCommand(in, now))
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ObserveWorkingTreeState(
	ctx context.Context,
	in SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.Send(ctx, syncCommand(in, now))
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: observe: %w", err)
	}
	return evt.Aggregate, nil
}

func syncCommand(in SyncInput, now time.Time) commands.SyncWorkingTreeState {
	return commands.SyncWorkingTreeState{
		ID:           in.ID,
		Added:        in.Added,
		Deleted:      in.Deleted,
		HasConflicts: in.HasConflicts,
		HasCommits:   in.HasCommits,
		Now:          now,
	}
}

func (w *workspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	got, err := w.ax.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
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
	evt, err := w.ax.SendWait(ctx, commands.SetMergeStrategy{ID: id, Strategy: strategy})
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
	evt, err := w.ax.SendWait(ctx, commands.SetLock{ID: id, Locked: locked, Protected: protected})
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
	evt, err := w.ax.SendWait(ctx, commands.Reparent{
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
	evt, err := w.ax.SendWait(ctx, commands.ResolveConflicts{
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
	evt, err := w.ax.SendWait(ctx, commands.UpdateForkPoint{ID: id, ForkPointSha: forkPointSha})
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
	evt, err := w.ax.SendWait(ctx, commands.ProvisionInPlace{
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
	evt, err := w.ax.SendWait(ctx, commands.ClearBranch{ID: id})
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
	evt, err := w.ax.SendWait(ctx, commands.RenameBranch{ID: id, Branch: branch})
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
	evt, err := w.ax.SendWait(ctx, commands.SetParentFromPR{ID: id, ParentID: parentID})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set parent from pr: %w", err)
	}
	return evt.Aggregate, nil
}

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

func (w *workspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := w.readModel.ListOrRebuild(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return rows, nil
}

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
