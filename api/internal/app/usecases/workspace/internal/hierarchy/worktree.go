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

// deriveWorktreePath returns <home>/projects/<project>/<slug>/<branch>/worktree for a branch. It rejects
// a workspace ROOT that collides case-insensitively with a sibling root (ErrInvalidArgument), comparing
// roots with roots via worktreepath.WorkspaceRoot.
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
	// The directory is named for the branch at creation and never follows a rename, so a frozen name may
	// belong to a workspace that moved on; take the next free variant rather than refuse.
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

// resolveSlug resolves the repo's on-disk identity slug. It always loads the repo row so a remote that
// encodes no identity can fall back to the repo NAME; a caller-supplied remoteURL is applied on top.
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

// addWorktree applies the checkout-vs-create decision and returns the fork point. An existing remote
// branch is checked out at origin's tip (the fork point is that tip); otherwise the branch is created
// from the parent's start point, which becomes the fork point.
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
	// Resolved only on the create path: the checkout path ignores it, and FetchRef is a real network fetch
	// under the contended per-clone repo lock.
	startPoint := u.parentStartPoint(ctx, in)
	startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, startPoint)
	if err != nil {
		return addedWorktree{}, fmt.Errorf("create child: worktree add: %w", err)
	}
	// `worktree add -b` refuses an existing branch, so success means the branch
	// is new — Crowbar's to delete with the workspace.
	return addedWorktree{startSha: startSha, createdBranch: true}, nil
}

// addedWorktree is what a successful worktree add reports: the fork point to
// record, and whether the branch was created by the add (and so is Crowbar's
// to delete later) or already existed (and so never is).
type addedWorktree struct {
	startSha      string
	createdBranch bool
}

// parentStartPoint resolves origin's FRESH tip of the parent, else the local parent ref. FetchRef, not a
// fast-forward, because git refuses to fetch into a branch checked out in any worktree, and protected
// parents always are. The parent never moves here; every failure degrades to the local tip and warns.
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

// branchIsRemoteBranch trusts the local origin/<branch> ref first, so the decision matches the branch
// list the user picked from; the live ls-remote fallback (failures read as false) must never veto a
// branch a local remote-tracking ref already confirms.
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

// checkoutRemoteBranch refreshes origin/<branch> and checks the worktree out AT that ref: importing
// means origin's branch, so a diverged local branch must not win. FetchRef only moves the remote ref,
// which git always allows; a failed fetch proceeds from a present local origin/<branch>, else is fatal.
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
	// Read BEFORE the reset moves it. No local branch is the one case where `-B`
	// creates it; a local branch that already existed stays the user's.
	localTip, err := provision.LocalTip(ctx, u.git, in.RepoPath, in.Branch)
	if err != nil {
		return addedWorktree{}, fmt.Errorf("create child: %w", err)
	}
	forkPoint, err := u.git.WorktreeAddAtRef(ctx, in.RepoPath, path, in.Branch, "origin/"+in.Branch)
	if err != nil {
		return addedWorktree{}, fmt.Errorf("create child: worktree checkout: %w", err)
	}
	provision.WarnOnDiscardedLocalTip(ctx, u.git, in.RepoPath, in.Branch, localTip, forkPoint)
	// `git worktree add -B` sets no upstream, so this alone makes the branch track origin/<branch> (the
	// ahead/behind contract). Best-effort: the content is already correct.
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
