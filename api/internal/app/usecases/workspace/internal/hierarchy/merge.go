package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// MergeResult is a local merge-into-parent's outcome. On conflict the merge is aborted (never a stuck
// worktree), the child shows pr-conflicts and ParentTipSha is empty; on success ParentTipSha is the
// parent's new HEAD, which the child's fork point has advanced to.
type MergeResult struct {
	ConflictsPending bool   `json:"conflictsPending"`
	ParentTipSha     string `json:"parentTipSha"`
}

func (u *hierarchyUsecase) MergeIntoParent(
	ctx context.Context,
	childID string,
	strategy gitdomain.MergeStrategy,
) (MergeResult, error) {
	child, parent, err := u.loadMergePair(ctx, childID)
	if err != nil {
		return MergeResult{}, err
	}
	if guardErr := u.guardMerge(ctx, child, parent, strategy); guardErr != nil {
		return MergeResult{}, guardErr
	}
	if mergeErr := u.runMerge(ctx, child, parent, strategy); mergeErr != nil {
		return u.handleMergeError(ctx, child, parent, strategy, mergeErr)
	}
	return u.finalizeMerge(ctx, child, parent)
}

func (u *hierarchyUsecase) loadMergePair(
	ctx context.Context,
	childID string,
) (domain.Workspace, domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, domain.Workspace{}, fmt.Errorf("merge: get child: %w", err)
	}
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if err != nil {
		return domain.Workspace{}, domain.Workspace{}, fmt.Errorf("merge: get parent: %w", err)
	}
	return child, parent, nil
}

func (u *hierarchyUsecase) guardMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
) error {
	if parent.Provisioning == domain.WorkspacePlaceholder {
		return ErrParentUnprovisioned
	}
	if parent.Status == domain.WorkspaceStatusLocked {
		return ErrParentLocked
	}
	if strategy != gitdomain.MergeStrategyRebase {
		return nil
	}
	hasKids, err := u.childHasChildren(ctx, child.ID)
	if err != nil {
		return fmt.Errorf("merge: leaf check: %w", err)
	}
	if hasKids {
		return ErrRebaseNonLeaf
	}
	return nil
}

func (u *hierarchyUsecase) runMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
) error {
	if strategy == gitdomain.MergeStrategySquash {
		subject := fmt.Sprintf("Squash merge %s", child.Branch)
		return u.git.MergeSquash(ctx, parent.WorktreePath, child.Branch, subject)
	}
	if strategy == gitdomain.MergeStrategyRebase {
		return u.runRebaseMerge(ctx, child, parent)
	}
	return u.git.MergeCommit(ctx, parent.WorktreePath, child.Branch)
}

func (u *hierarchyUsecase) runRebaseMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
) error {
	return u.git.RebaseThenFFMerge(
		ctx,
		child.WorktreePath,
		parent.Branch,
		parent.WorktreePath,
		child.Branch,
	)
}

func (u *hierarchyUsecase) handleMergeError(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
	strategy gitdomain.MergeStrategy,
	mergeErr error,
) (MergeResult, error) {
	if !errors.Is(mergeErr, enginegit.ErrConflict) {
		return MergeResult{}, fmt.Errorf("merge: run: %w", mergeErr)
	}
	// A conflicting merge must never leave a worktree stuck: abort the op where it runs (parent for
	// squash/merge, child for rebase) and surface the conflict as the child's pr-conflicts state.
	abortPath := parent.WorktreePath
	abortWS := parent.ID
	if strategy == gitdomain.MergeStrategyRebase {
		abortPath = child.WorktreePath
		abortWS = child.ID
	}
	if abortErr := u.git.OperationAbort(ctx, abortPath); abortErr != nil { //nolint:nestif // stuck-worktree recovery after a failed conflict abort; flattening risks the recovery path
		slog.WarnContext(ctx, "merge: abort after conflict failed; worktree may be stuck",
			"workspace_id", child.ID, "abort_path", abortPath, "err", abortErr)
		// The abort failed, so that worktree is still mid-merge: flag the parent pr-conflicts too, so the stuck
		// state is visible and recoverable rather than reported as success.
		if abortWS != child.ID {
			if _, perr := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
				ID:           abortWS,
				HasConflicts: true,
			}, u.now()); perr != nil {
				slog.WarnContext(ctx, "merge: flag stuck parent after failed abort",
					"workspace_id", abortWS, "err", perr)
			}
		}
	}
	// A local merge/rebase conflict transitions the child to Status=pr-conflicts:
	// the HasConflicts sync input drives the status enum.
	_, err := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           child.ID,
		HasConflicts: true,
	}, u.now())
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge: set conflicts: %w", err)
	}
	return MergeResult{ConflictsPending: true}, nil
}

func (u *hierarchyUsecase) finalizeMerge(
	ctx context.Context,
	child domain.Workspace,
	parent domain.Workspace,
) (MergeResult, error) {
	tip, err := u.git.RevParse(ctx, parent.WorktreePath, "HEAD")
	if err != nil {
		return MergeResult{}, fmt.Errorf("merge: parent tip: %w", err)
	}
	if _, err := u.workspaces.UpdateForkPoint(ctx, child.ID, tip); err != nil {
		return MergeResult{}, fmt.Errorf("merge: update fork point: %w", err)
	}
	if _, err := u.resyncSummary(ctx, parent.ID, parent.WorktreePath, parent.ForkPointSha, false); err != nil {
		slog.WarnContext(ctx, "merge: parent summary resync failed; read-model will self-correct", "workspace_id", parent.ID, "err", err)
	}
	if _, err := u.resyncSummary(ctx, child.ID, child.WorktreePath, tip, false); err != nil {
		slog.WarnContext(ctx, "merge: child summary resync failed; read-model will self-correct", "workspace_id", child.ID, "err", err)
	}
	return MergeResult{ParentTipSha: tip}, nil
}

// resyncSummary recomputes a workspace's summary after a merge; conflicted marks a conflict git no
// longer shows. forkPointSha is a freshly computed post-op SHA used as the diff base verbatim, and is
// superseded on the next branch-based recompute.
func (u *hierarchyUsecase) resyncSummary(
	ctx context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
	conflicted bool,
) (domain.Workspace, error) {
	added, deleted, hasConflicts, hasCommits, err := u.git.WorkingTreeSummary(ctx, worktreePath, forkPointSha)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("merge: resync summary: %w", err)
	}
	ws, err := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           id,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts || conflicted,
		HasCommits:   hasCommits,
	}, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("merge: resync summary: sync: %w", err)
	}
	return ws, nil
}
