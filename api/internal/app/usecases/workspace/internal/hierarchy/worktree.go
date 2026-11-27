package hierarchy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/provision"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func (u *hierarchyUsecase) deriveWorktreePath(
	ctx context.Context,
	home string,
	projectID string,
	repoID string,
	remoteURL string,
	branch string,
) (string, error) {
	slug, err := u.resolveSlug(ctx, repoID, remoteURL)
	if err != nil {
		return "", fmt.Errorf("resolve worktree slug: %w", err)
	}
	siblings, err := worktreepath.SiblingRoots(home, projectID, slug)
	if err != nil {
		return "", fmt.Errorf("scan sibling worktrees: %w", err)
	}
	pathBranch, err := worktreepath.FreePathBranch(home, projectID, slug, branch, siblings)
	if err != nil {
		return "", fmt.Errorf("%w: %w", problem.ErrInvalidArgument, err)
	}
	path, err := worktreepath.Derive(home, projectID, slug, pathBranch)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (u *hierarchyUsecase) resolveSlug(
	ctx context.Context,
	repoID string,
	remoteURL string,
) (string, error) {
	repo, err := u.repos.FindByKey(ctx, repoID)
	if err != nil {
		return "", err
	}
	if repo == nil {
		return "", problem.ErrNotFound
	}
	if remoteURL != "" {
		repo.RemoteURL = remoteURL
	}
	return worktreepath.RemoteSlug(*repo), nil
}

func (u *hierarchyUsecase) addWorktree(
	ctx context.Context,
	in CreateChildInput,
	path string,
) (addedWorktree, error) {
	onRemote, err := u.branchIsRemoteBranch(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return addedWorktree{}, err
	}
	if onRemote {
		return u.checkoutRemoteBranch(ctx, in, path)
	}
	startPoint := u.parentStartPoint(ctx, in)
	startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, startPoint)
	if err != nil {
		return addedWorktree{}, fmt.Errorf("create child: worktree add: %w", err)
	}
	return addedWorktree{startSha: startSha, createdBranch: true}, nil
}

type addedWorktree struct {
	startSha      string
	createdBranch bool
}

func (u *hierarchyUsecase) parentStartPoint(
	ctx context.Context,
	in CreateChildInput,
) string {
	if in.ParentBranch == "" || in.FromParentHead {
		return u.detachedParentHead(ctx, in)
	}
	parentOnRemote, err := u.git.RemoteBranchExists(ctx, in.RepoPath, in.ParentBranch)
	if err != nil || !parentOnRemote {
		return in.ParentBranch
	}
	if fetchErr := u.git.FetchRef(ctx, in.RepoPath, in.ParentBranch); fetchErr != nil {
		slog.WarnContext(ctx, "create child: could not fetch parent from origin; branching from local tip",
			"parent", in.ParentBranch, "err", fetchErr)
		return in.ParentBranch
	}
	remoteTip, rpErr := u.git.RevParse(ctx, in.RepoPath, "origin/"+in.ParentBranch)
	if rpErr != nil {
		slog.WarnContext(ctx, "create child: origin parent ref unresolved after fetch; branching from local tip",
			"parent", in.ParentBranch, "err", rpErr)
		return in.ParentBranch
	}
	return remoteTip
}

func (u *hierarchyUsecase) branchIsRemoteBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	tracking, err := u.git.RemoteTrackingBranchExists(ctx, repoPath, branch)
	if err != nil {
		return false, fmt.Errorf("create child: remote-tracking branch exists: %w", err)
	}
	if tracking {
		return true, nil
	}
	remote, err := u.git.RemoteBranchExists(ctx, repoPath, branch)
	if err != nil {
		return false, fmt.Errorf("create child: remote branch exists: %w", err)
	}
	return remote, nil
}

func (u *hierarchyUsecase) checkoutRemoteBranch(
	ctx context.Context,
	in CreateChildInput,
	path string,
) (addedWorktree, error) {
	if fetchErr := u.git.FetchRef(ctx, in.RepoPath, in.Branch); fetchErr != nil {
		tracking, trErr := u.git.RemoteTrackingBranchExists(ctx, in.RepoPath, in.Branch)
		if trErr != nil || !tracking {
			return addedWorktree{}, fmt.Errorf("create child: fetch origin branch: %w", fetchErr)
		}
		slog.WarnContext(ctx, "create child: could not fetch origin branch; checking out from local remote-tracking ref",
			"branch", in.Branch, "err", fetchErr)
	}
	localTip, err := provision.LocalTip(ctx, u.git, in.RepoPath, in.Branch)
	if err != nil {
		return addedWorktree{}, fmt.Errorf("create child: %w", err)
	}
	forkPoint, err := u.git.WorktreeAddAtRef(ctx, in.RepoPath, path, in.Branch, "origin/"+in.Branch)
	if err != nil {
		return addedWorktree{}, fmt.Errorf("create child: worktree checkout: %w", err)
	}
	provision.WarnOnDiscardedLocalTip(ctx, u.git, in.RepoPath, in.Branch, localTip, forkPoint)
	if err := u.git.SetUpstream(ctx, in.RepoPath, in.Branch); err != nil {
		slog.WarnContext(ctx, "create child: could not set upstream on imported branch; content is correct but ahead/behind may not report",
			"branch", in.Branch, "err", err)
	}
	return addedWorktree{startSha: forkPoint, createdBranch: localTip == ""}, nil
}

func (u *hierarchyUsecase) repoPathFor(
	ctx context.Context,
	ws domain.Workspace,
) (string, error) {
	repo, err := u.repos.FindByKey(ctx, ws.RepoID)
	if err != nil {
		return "", fmt.Errorf("repo path: %w", err)
	}
	if repo == nil {
		return "", fmt.Errorf("worktree: repo %s not found", ws.RepoID)
	}
	return repo.Path, nil
}
