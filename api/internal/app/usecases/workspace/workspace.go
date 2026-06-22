package workspace

import (
	"context"
	"fmt"
	"time"

	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceLifecycleRepo is the workspace repository surface the usecase needs.
type WorkspaceLifecycleRepo interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		in wsrepo.SyncInput,
		now time.Time,
	) (domain.Workspace, error)
}

// WorkingTreeGitEngine is the git surface used to recompute the summary and to
// predict whether folding a child into its parent would conflict.
type WorkingTreeGitEngine interface {
	WorkingTreeSummary(
		ctx context.Context,
		repoPath string,
		forkPointSha string,
	) (added, deleted int, hasConflicts, hasCommits bool, err error)
	WouldMergeConflict(
		ctx context.Context,
		repoPath string,
		ours string,
		theirs string,
	) (bool, error)
}

// ProjectActivityRollup is the best-effort project lastActivity roll-up surface.
type ProjectActivityRollup interface {
	TouchProjectActivity(
		ctx context.Context,
		repoID string,
		now time.Time,
	)
}

// Usecase is the lifecycle/read surface over the workspace aggregate.
// SyncWorkingTreeState is the shared wrapper the watcher, file, and git usecases
// call to recompute and persist the working-tree summary (00 §5.3).
type Usecase interface {
	// List returns every workspace row from the read model.
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)

	// Get returns the workspace with the given id.
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)

	// SetMergeStrategy writes the mergeStrategy field (used by review PATCH).
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)

	// SyncWorkingTreeState recomputes the working-tree summary from git, issues
	// the sync command, and rolls up project lastActivity best-effort.
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)

	// MergeEligibilityFor resolves whether ws can be merged into its local
	// parent, reading the parent's status from the caller-held sibling set. The
	// ctx scopes the predicted-conflict git dry-run.
	MergeEligibilityFor(
		ctx context.Context,
		ws domain.Workspace,
		siblings []domain.Workspace,
	) MergeEligibility
}

type workspaceUsecase struct {
	repo   WorkspaceLifecycleRepo
	git    WorkingTreeGitEngine
	rollup ProjectActivityRollup
}

// New builds a Usecase from the workspace repo, the git engine summary surface,
// and the project roll-up usecase.
func New(
	repo WorkspaceLifecycleRepo,
	git WorkingTreeGitEngine,
	rollup ProjectActivityRollup,
) Usecase {
	return &workspaceUsecase{
		repo:   repo,
		git:    git,
		rollup: rollup,
	}
}

// List returns every workspace row from the read model.
func (u *workspaceUsecase) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	list, err := u.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list: %w", err)
	}
	return list, nil
}

// Get returns the workspace with the given id.
func (u *workspaceUsecase) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: get: %w", err)
	}
	return ws, nil
}

// SetMergeStrategy writes the mergeStrategy field (used by review PATCH).
func (u *workspaceUsecase) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	ws, err := u.repo.SetMergeStrategy(ctx, id, strategy)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set merge strategy: %w", err)
	}
	return ws, nil
}

// SyncWorkingTreeState recomputes the working-tree summary from git, issues the
// sync command, and rolls up project lastActivity best-effort.
func (u *workspaceUsecase) SyncWorkingTreeState(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	ws, err := u.repo.Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync working tree: get: %w", err)
	}
	in, err := u.summarize(ctx, ws)
	if err != nil {
		return domain.Workspace{}, err
	}
	synced, err := u.repo.SyncWorkingTreeState(ctx, in, now)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync working tree: %w", err)
	}
	u.rollup.TouchProjectActivity(ctx, ws.RepoID, now)
	return synced, nil
}

// MergeEligibilityFor resolves whether ws can be merged into its local parent.
// No repository call is made — the parent is resolved from siblings, which the
// caller already holds from a preceding List call. Delegates to
// ResolveMergeEligibility so the snapshot read and the live broadcast share one
// implementation; the caller supplies the context for the conflict dry-run.
func (u *workspaceUsecase) MergeEligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
	siblings []domain.Workspace,
) MergeEligibility {
	return ResolveMergeEligibility(ctx, ws, siblings, u.git)
}

func (u *workspaceUsecase) summarize(
	ctx context.Context,
	ws domain.Workspace,
) (wsrepo.SyncInput, error) {
	added, deleted, hasConflicts, hasCommits, err := u.git.WorkingTreeSummary(
		ctx,
		ws.WorktreePath,
		ws.ForkPointSha,
	)
	if err != nil {
		return wsrepo.SyncInput{}, fmt.Errorf("workspace: sync working tree: summary: %w", err)
	}
	return wsrepo.SyncInput{
		ID:           ws.ID,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	}, nil
}
