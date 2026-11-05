package hierarchy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func (u *hierarchyUsecase) Reparent(
	ctx context.Context,
	childID string,
	newParentID string,
) (domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get child: %w", err)
	}
	newParent, err := u.workspaces.Get(ctx, newParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get new parent: %w", err)
	}
	if guardErr := u.guardReparent(ctx, child, newParent); guardErr != nil {
		return domain.Workspace{}, guardErr
	}
	tip, err := u.git.RevParse(ctx, newParent.WorktreePath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: new parent tip: %w", err)
	}
	oldParentBranch, err := u.parentBranch(ctx, child)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: %w", err)
	}
	base, err := u.replayBase(ctx, child, oldParentBranch, tip)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: %w", err)
	}
	return u.replayAndReparent(ctx, child, newParentID, tip, base)
}

// parentBranch is the branch of child's current parent: "" for a root, and for
// a parent row that is gone, so an orphaned child can still be moved.
func (u *hierarchyUsecase) parentBranch(
	ctx context.Context,
	child domain.Workspace,
) (string, error) {
	if child.ParentID == "" {
		return "", nil
	}
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if problem.CodeOf(err) == problem.NotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get parent: %w", err)
	}
	return parent.Branch, nil
}

// replayBase is where a rebase of child starts: its merge-base with the parent's
// branch (with fallback when it has none or it no longer resolves), never the
// recorded fork point. It drops nothing the parent lacks; it may replay more.
func (u *hierarchyUsecase) replayBase(
	ctx context.Context,
	child domain.Workspace,
	parentBranch string,
	fallback string,
) (string, error) {
	if parentBranch != "" {
		if base, err := u.mergeBase(ctx, child, parentBranch); err == nil {
			return base, nil
		}
	}
	return u.mergeBase(ctx, child, fallback)
}

func (u *hierarchyUsecase) mergeBase(
	ctx context.Context,
	child domain.Workspace,
	ref string,
) (string, error) {
	base, err := u.git.MergeBase(ctx, child.WorktreePath, ref, child.Branch)
	if err != nil {
		return "", fmt.Errorf("replay base: merge-base: %w", err)
	}
	return base, nil
}

// replayAndReparent rebases the child onto the new parent's tip, then settles the move.
// A conflict is aborted back to a clean worktree, but the child still moves and is marked
// pr-conflicts until the user runs RebaseOntoParent.
func (u *hierarchyUsecase) replayAndReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	tip string,
	base string,
) (domain.Workspace, error) {
	rebaseErr := u.git.RebaseOnto(ctx, child.WorktreePath, tip, base, child.Branch)
	if rebaseErr == nil {
		// Clean: the branch is genuinely based on the new parent's tip now.
		return u.settleReparent(ctx, child, newParentID, tip, false)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("reparent: rebase onto: %w", rebaseErr)
	}
	// Conflict: abort the rebase so the worktree returns to clean (never the stuck
	// mid-rebase mess), but keep the move.
	if abortErr := u.git.OperationAbort(ctx, child.WorktreePath); abortErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: abort after conflict: %w", abortErr)
	}
	// Fork point = merge-base of the branch and the new parent, so diffs/summaries
	// read against the new parent's lineage until a clean rebase finalizes it.
	onPaper, baseErr := u.git.MergeBase(ctx, child.WorktreePath, tip, child.Branch)
	if baseErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: merge-base: %w", baseErr)
	}
	return u.settleReparent(ctx, child, newParentID, onPaper, true)
}

// settleReparent persists the child's new parent + fork point and resyncs its
// working-tree summary so the diff stats reflect the new base. The resync is
// best-effort unless the rebase conflicted: that is the user's only signal.
func (u *hierarchyUsecase) settleReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	forkPoint string,
	conflicted bool,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Reparent(ctx, child.ID, newParentID, forkPoint, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: persist: %w", err)
	}
	synced, syncErr := u.resyncSummary(ctx, child.ID, child.WorktreePath, forkPoint, conflicted)
	if syncErr == nil {
		return synced, nil
	}
	if conflicted {
		return domain.Workspace{}, fmt.Errorf("reparent: moved under %s, but marking its rebase conflict failed: %w", newParentID, syncErr)
	}
	slog.WarnContext(ctx, "reparent: child summary resync failed; read-model will self-correct",
		"workspace_id", child.ID, "err", syncErr)
	return ws, nil
}

// RebaseOntoParent finishes a moved-but-conflicting child's move by rebasing it onto its
// current parent's tip, keeping a conflicting rebase in progress for the user to resolve.
// The parent tip is persisted as fork point up front so the branch reads right once resolved.
func (u *hierarchyUsecase) RebaseOntoParent(
	ctx context.Context,
	childID string,
) (domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: get child: %w", err)
	}
	if child.ParentID == "" {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: workspace has no parent")
	}
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: get parent: %w", err)
	}
	if parent.Provisioning == domain.WorkspacePlaceholder {
		return domain.Workspace{}, ErrParentUnprovisioned
	}
	tip, err := u.git.RevParse(ctx, parent.WorktreePath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: parent tip: %w", err)
	}
	base, err := u.replayBase(ctx, child, parent.Branch, tip)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: %w", err)
	}
	rebaseErr := u.git.RebaseOnto(ctx, child.WorktreePath, tip, base, child.Branch)
	if rebaseErr == nil {
		// Clean: the child is genuinely integrated onto the parent.
		return u.settleReparent(ctx, child, child.ParentID, tip, false)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: %w", rebaseErr)
	}
	// Conflict: KEEP the rebase in progress (the user resolves it with the
	// standard conflict tooling). Persist the intended fork point now so the
	// branch reads correctly once resolved, and surface the conflict via status.
	if _, err := u.workspaces.Reparent(ctx, child.ID, child.ParentID, tip, u.now()); err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: persist: %w", err)
	}
	ws, err := u.workspaces.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:           child.ID,
		HasConflicts: true,
	}, u.now())
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: set conflicts: %w", err)
	}
	return ws, nil
}
