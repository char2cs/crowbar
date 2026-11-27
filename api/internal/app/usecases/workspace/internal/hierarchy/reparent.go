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

func (u *hierarchyUsecase) replayAndReparent(
	ctx context.Context,
	child domain.Workspace,
	newParentID string,
	tip string,
	base string,
) (domain.Workspace, error) {
	rebaseErr := u.git.RebaseOnto(ctx, child.WorktreePath, tip, base, child.Branch)
	if rebaseErr == nil {
		return u.settleReparent(ctx, child, newParentID, tip, false)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("reparent: rebase onto: %w", rebaseErr)
	}
	if abortErr := u.git.OperationAbort(ctx, child.WorktreePath); abortErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: abort after conflict: %w", abortErr)
	}
	onPaper, baseErr := u.git.MergeBase(ctx, child.WorktreePath, tip, child.Branch)
	if baseErr != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: merge-base: %w", baseErr)
	}
	return u.settleReparent(ctx, child, newParentID, onPaper, true)
}

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
		return u.settleReparent(ctx, child, child.ParentID, tip, false)
	}
	if !errors.Is(rebaseErr, enginegit.ErrConflict) {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: %w", rebaseErr)
	}
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
