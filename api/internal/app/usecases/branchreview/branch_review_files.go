package branchreview

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sync/singleflight"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func (u *branchReviewUsecase) GetFiles(
	ctx context.Context,
	wsID string,
	commit string,
) ([]gitdomain.ReviewFileSummary, error) {
	read, err := u.scopeOf(ctx, wsID, commit, domain.Workspace{})
	if err != nil {
		return nil, err
	}
	return read.scope.Files, nil
}

func (u *branchReviewUsecase) GetScope(
	ctx context.Context,
	ws domain.Workspace,
) (gitdomain.ReviewScope, error) {
	read, err := u.scopeOf(ctx, ws.ID, "", ws)
	if err != nil {
		return gitdomain.ReviewScope{}, err
	}
	outline, err := u.outlineForScope(ctx, ws, read)
	if err != nil {
		return gitdomain.ReviewScope{}, err
	}
	read.scope.Outline = outline
	return read.scope, nil
}

func (u *branchReviewUsecase) outlineForScope(
	ctx context.Context,
	ws domain.Workspace,
	read scopeRead,
) ([]gitdomain.FileOutline, error) {
	key, cacheable := u.branchOutlineKey(ctx, ws, read.scope.Base, read.treeClean)
	return u.outlineWithKey(ctx, ws, read.scope.Base, key, cacheable)
}

type scopeRead struct {
	scope     gitdomain.ReviewScope
	treeClean bool
}

func (u *branchReviewUsecase) scopeOf(
	ctx context.Context,
	wsID string,
	commit string,
	known domain.Workspace,
) (scopeRead, error) {
	shared := u.fileReads.DoChan(wsID+"\x00"+commit, func() (any, error) {
		return u.readScope(context.WithoutCancel(ctx), wsID, commit, known)
	})
	select {
	case <-ctx.Done():
		return scopeRead{}, ctx.Err()
	case res := <-shared:
		return sharedScope(res)
	}
}

func sharedScope(
	res singleflight.Result,
) (scopeRead, error) {
	if res.Err != nil {
		return scopeRead{}, res.Err
	}
	read, _ := res.Val.(scopeRead)
	read.scope.Files = slices.Clone(read.scope.Files)
	return read, nil
}

func (u *branchReviewUsecase) readScope(
	ctx context.Context,
	wsID string,
	commit string,
	known domain.Workspace,
) (scopeRead, error) {
	ws, err := u.workspaceFor(ctx, wsID, known)
	if err != nil {
		return scopeRead{}, err
	}
	ref, err := u.resolveScopeRef(ctx, ws, commit)
	if err != nil {
		return scopeRead{}, fmt.Errorf("branch review: resolve ref: %w", err)
	}
	files, clean, err := u.filesForRef(ctx, ws, ref, commit)
	if err != nil {
		return scopeRead{}, err
	}
	return scopeRead{scope: gitdomain.ReviewScope{Base: ref, Files: files}, treeClean: clean}, nil
}

func (u *branchReviewUsecase) workspaceFor(
	ctx context.Context,
	wsID string,
	known domain.Workspace,
) (domain.Workspace, error) {
	if known.ID != "" && known.ID == wsID {
		return known, nil
	}
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("branch review: get workspace: %w", asNotFound(err))
	}
	return ws, nil
}

func (u *branchReviewUsecase) filesForRef(
	ctx context.Context,
	ws domain.Workspace,
	ref string,
	commit string,
) ([]gitdomain.ReviewFileSummary, bool, error) {
	if isCommitScoped(commit) {
		files, err := u.git.ReviewFiles(ctx, ws.WorktreePath, ref, nil)
		if err != nil {
			return nil, false, fmt.Errorf("branch review: review files: %w", err)
		}
		return files, false, nil
	}
	status, statusErr := u.git.Status(ctx, ws.WorktreePath)
	files, err := u.git.ReviewFiles(ctx, ws.WorktreePath, ref, dirtyPaths(status, statusErr))
	if err != nil {
		return nil, false, fmt.Errorf("branch review: review files: %w", err)
	}
	clean := statusErr == nil && !hasTrackedChanges(status)
	if statusErr != nil {
		return files, clean, nil
	}
	return mergeWorkingTree(status, files), clean, nil
}

func dirtyPaths(
	status gitdomain.GitStatus,
	err error,
) []string {
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(status.Files))
	for _, f := range status.Files {
		out = append(out, f.Path)
	}
	return out
}

func mergeWorkingTree(
	status gitdomain.GitStatus,
	files []gitdomain.ReviewFileSummary,
) []gitdomain.ReviewFileSummary {
	dirty, staged := workingTreeIndex(status)
	for i := range files {
		files[i].Uncommitted = dirty[files[i].Path]
		files[i].Staged = staged[files[i].Path]
	}
	return append(files, untrackedSummaries(status, files)...)
}

func workingTreeIndex(
	status gitdomain.GitStatus,
) (map[string]bool, map[string]bool) {
	dirty := make(map[string]bool, len(status.Files))
	staged := make(map[string]bool, len(status.Files))
	for _, f := range status.Files {
		dirty[f.Path] = true
		staged[f.Path] = staged[f.Path] || f.Staged
	}
	return dirty, staged
}

func untrackedSummaries(
	status gitdomain.GitStatus,
	existing []gitdomain.ReviewFileSummary,
) []gitdomain.ReviewFileSummary {
	present := make(map[string]bool, len(existing))
	for _, f := range existing {
		present[f.Path] = true
	}
	var out []gitdomain.ReviewFileSummary
	for _, f := range status.Files {
		if !isNewUntracked(f, present) {
			continue
		}
		present[f.Path] = true
		out = append(out, gitdomain.ReviewFileSummary{
			Path:        f.Path,
			Status:      gitdomain.GitFileStatusUntracked,
			Uncommitted: true,
		})
	}
	return out
}

func isNewUntracked(
	f gitdomain.GitFile,
	present map[string]bool,
) bool {
	return f.Status == gitdomain.GitFileStatusUntracked && !present[f.Path]
}
