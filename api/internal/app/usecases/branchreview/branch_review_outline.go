package branchreview

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func (u *branchReviewUsecase) GetOutline(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.FileOutline, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return nil, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	key, cacheable := u.cacheableOutlineKey(ctx, ws, ref, commit)
	return u.outlineWithKey(ctx, ws, ref, key, cacheable)
}

func (u *branchReviewUsecase) outlineWithKey(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	key outlineKey,
	cacheable bool,
) ([]gitdomain.FileOutline, error) {
	if !cacheable {
		return u.readOutline(ctx, ws, ref)
	}
	return u.outlines.get(key, func() ([]gitdomain.FileOutline, error) {
		return u.readOutline(ctx, ws, ref)
	})
}

func (u *branchReviewUsecase) readOutline(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
) ([]gitdomain.FileOutline, error) {
	files, err := u.git.ReviewOutline(ctx, ws.WorktreePath, ref)
	if err != nil {
		return nil, fmt.Errorf("branch review: review outline: %w", err)
	}
	return files, nil
}

func (u *branchReviewUsecase) cacheableOutlineKey(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	commit string,
) (outlineKey, bool) {
	if isCommitScoped(commit) {
		return outlineKey{repoPath: ws.WorktreePath, ref: ref}, true
	}
	status, err := u.git.Status(ctx, ws.WorktreePath)
	if err != nil {
		return outlineKey{}, false
	}
	return u.branchOutlineKey(ctx, ws, ref, !hasTrackedChanges(status))
}

func (u *branchReviewUsecase) branchOutlineKey(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	treeClean bool,
) (outlineKey, bool) {
	if !treeClean {
		return outlineKey{}, false
	}
	head, err := u.git.RevParse(ctx, ws.WorktreePath, "HEAD")
	if err != nil || head == "" {
		return outlineKey{}, false
	}
	return outlineKey{repoPath: ws.WorktreePath, ref: ref, headSHA: head}, true
}

func hasTrackedChanges(
	status gitdomain.GitStatus,
) bool {
	for _, f := range status.Files {
		if f.Status != gitdomain.GitFileStatusUntracked {
			return true
		}
	}
	return false
}
