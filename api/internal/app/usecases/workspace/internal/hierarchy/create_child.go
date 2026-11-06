package hierarchy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateChildInput carries the fields needed to create a worktree-backed child. Repo fields are
// required only without a ParentID; leaving both RepoID and RepoPath blank inherits the parent's repo,
// while a caller that names its own repo (even a path-less one) keeps the explicit create.
type CreateChildInput struct {
	RepoID       string
	ProjectID    string
	RepoPath     string
	RemoteURL    string // git remote origin URL; required for worktree path derivation
	Branch       string
	ParentID     string
	ParentBranch string
	// OwnWorktree nil inherits whether the parent owns a worktree; a set value always wins.
	OwnWorktree *bool
	// ForceLocked marks the workspace locked regardless of branch protection, for a workspace the caller
	// knows is immutable (a default-branch workspace adopted without a git provider).
	ForceLocked    bool
	parentWorktree string // the inherited parent's checkout: a branchless parent forks from its HEAD
}

//nolint:gocyclo // orchestrates create with intricate worktree/branch rollback paths; splitting risks the rollback invariants
func (u *hierarchyUsecase) CreateChild(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	in, ownWorktree, err := u.resolveInherited(ctx, in)
	if err != nil {
		return domain.Workspace{}, err
	}
	// Repos with no on-disk path (e.g. virtual/test repos), and a create that
	// does not own a worktree (explicitly, or defaulted off a workspace-less
	// parent), skip all git operations and create a workspace row directly.
	if in.RepoPath == "" || !ownWorktree {
		return u.createDirectRow(ctx, in)
	}
	// A spontaneous create leaves Branch blank: naming happens server-side because only this side can
	// collision-check against real refs.
	if in.Branch == "" {
		branch, genErr := u.generateBranchName(ctx, in.RepoPath)
		if genErr != nil {
			return domain.Workspace{}, fmt.Errorf("create child: %w", genErr)
		}
		in.Branch = branch
	}
	// At most one MANAGED (non-default) workspace per (repo, branch). The default
	// workspace (the imported repo folder) never counts — see branchWorkspaceExists.
	exists, err := u.branchWorkspaceExists(ctx, in.RepoID, in.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	if exists {
		return domain.Workspace{}, fmt.Errorf("%w (repo %s, branch %q)", ErrBranchWorkspaceExists, in.RepoID, in.Branch)
	}
	// The FIRST create on the default branch with no parent adopts the repo folder as the default
	// workspace; a repeat must fall through to a managed worktree, where git rejects a second checkout.
	if in.ParentID == "" && in.Branch == in.ParentBranch { //nolint:nestif // adopt-vs-managed create branching; flattening risks the adoption invariant
		_, adopted, err := u.repoHomeWorkspaceID(ctx, in.RepoID)
		if err != nil {
			return domain.Workspace{}, fmt.Errorf("create child: list workspaces: %w", err)
		}
		if !adopted {
			return u.adoptMainWorktree(ctx, in)
		}
	}
	wsID := uuid.NewString()
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: crowbar home: %w", err)
	}
	// Resolve the locked flag BEFORE creating the worktree so a provider hiccup
	// can't leave an orphaned worktree+branch on disk (it has no on-disk effect).
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: locked: %w", err)
	}
	path, err := u.deriveWorktreePath(ctx, home, in.ProjectID, in.RepoID, in.RemoteURL, in.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	added, detached, err := u.addWorktreeFreeingHome(ctx, in, path, home)
	if err != nil {
		return domain.Workspace{}, err
	}
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		ID:            wsID,
		RepoID:        in.RepoID,
		ProjectID:     in.ProjectID,
		Branch:        in.Branch,
		WorktreePath:  path,
		ForkPointSha:  added.startSha,
		ParentID:      in.ParentID,
		Protected:     locked || in.ForceLocked,
		CreatedBranch: added.createdBranch,
		Provisioning:  domain.WorkspaceProvisioned,
	}, u.now())
	if err != nil {
		// The worktree + branch are on disk but the workspace row never landed.
		// Clean them up best-effort so a fresh-wsID retry isn't blocked by the
		// orphaned branch and the worktree dir doesn't dangle forever.
		u.cleanupFailedWorktree(ctx, in.RepoPath, path, in.Branch, detached, added.createdBranch)
		return domain.Workspace{}, err
	}
	if nErr := u.mintWorkspaceNode(ctx, ws.ID); nErr != nil {
		// The row landed but its Node position could not be minted: roll everything back, row included, since
		// a Workspace must never survive without its own Node row.
		u.cleanupFailedWorktree(ctx, in.RepoPath, path, in.Branch, detached, added.createdBranch)
		u.discardWorkspaceRow(ctx, ws.ID, "create child")
		return domain.Workspace{}, nErr
	}
	// The worktree took the branch off the repo home, so the home row must stop claiming it. Runs only past
	// every rollback, so the clear never outlives a detach that is unwound.
	u.clearDetachedHomeBranch(ctx, detached, in.RepoID, in.Branch)
	return ws, nil
}

// addWorktreeFreeingHome adds the worktree, and when git refuses because the repo home holds the branch
// (its branch is incidental) detaches the home and retries once; a managed holder is left to fail. On
// failure a detach is undone; detached reports one that stands.
func (u *hierarchyUsecase) addWorktreeFreeingHome(
	ctx context.Context,
	in CreateChildInput,
	path string,
	home string,
) (addedWorktree, bool, error) {
	added, err := u.addWorktree(ctx, in, path)
	if err == nil {
		return added, false, nil
	}
	outcome, hErr := holder.Resolve(ctx, u.git, in.RepoPath, in.Branch, home)
	if hErr != nil || outcome.Kind != holder.HeldByHome {
		return addedWorktree{}, false, err
	}
	if dErr := u.git.DetachWorktree(ctx, in.RepoPath); dErr != nil {
		return addedWorktree{}, false, err
	}
	added, err = u.addWorktree(ctx, in, path)
	if err != nil {
		u.reattachMain(ctx, true, in.RepoPath, in.Branch)
		return addedWorktree{}, false, err
	}
	return added, true, nil
}

// clearDetachedHomeBranch blanks the home row's branch after a create detached the home to claim it.
// Best-effort: the worktree and its row are already committed.
func (u *hierarchyUsecase) clearDetachedHomeBranch(
	ctx context.Context,
	detached bool,
	repoID string,
	branch string,
) {
	if !detached {
		return
	}
	homeID, ok, err := u.repoHomeWorkspaceID(ctx, repoID)
	if err != nil || !ok {
		slog.WarnContext(ctx, "create child: could not find the repo home to clear its branch",
			"repo", repoID, "branch", branch, "err", err)
		return
	}
	if _, cErr := u.workspaces.ClearBranch(ctx, homeID); cErr != nil {
		slog.WarnContext(ctx, "create child: could not clear the detached home's branch",
			"repo", repoID, "branch", branch, "err", cErr)
	}
}

// createDirectRow creates the row with no git work, for a path-less repo or a create that owns no
// worktree. A failed Node mint rolls the row back; there is no worktree to unwind.
func (u *hierarchyUsecase) createDirectRow(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		ID:        uuid.NewString(),
		RepoID:    in.RepoID,
		ProjectID: in.ProjectID,
		Branch:    in.Branch,
		ParentID:  in.ParentID,
		Protected: in.ForceLocked,
		// No checkout of its own: nothing may run git in it until one exists.
		Provisioning: domain.WorkspacePlaceholder,
	}, u.now())
	if err != nil {
		return domain.Workspace{}, err
	}
	if nErr := u.mintWorkspaceNode(ctx, ws.ID); nErr != nil {
		u.discardWorkspaceRow(ctx, ws.ID, "create child")
		return domain.Workspace{}, nErr
	}
	return ws, nil
}

// cleanupFailedWorktree removes a worktree whose row failed to land, reattaching a detached main folder
// first and deleting the branch only when this create made it. Best-effort: the original failure is
// what the caller reports.
func (u *hierarchyUsecase) cleanupFailedWorktree(
	ctx context.Context,
	repoPath string,
	path string,
	branch string,
	detached bool,
	createdBranch bool,
) {
	// The undo outlives the request: a caller that hung up would otherwise
	// leave the worktree and branch on disk for good.
	ctx = context.WithoutCancel(ctx)
	// The worktree was created a moment ago by this very call, so nothing in
	// it can be anyone's uncommitted work: force is safe.
	if rmErr := u.git.WorktreeRemove(ctx, repoPath, path, true); rmErr != nil {
		slog.WarnContext(ctx, "create child: cleanup worktree after failed create",
			"worktree", path, "err", rmErr)
	}
	u.reattachMain(ctx, detached, repoPath, branch)
	if !createdBranch {
		return
	}
	if delErr := u.git.ForceDeleteBranch(ctx, repoPath, branch); delErr != nil {
		slog.WarnContext(ctx, "create child: cleanup branch after failed create",
			"branch", branch, "err", delErr)
	}
}

// resolveInherited fills blank repo fields from the parent's repo and reports the effective
// OwnWorktree. The lookup runs only when both RepoID and RepoPath are blank, so a caller that knows its
// own repo is never silently swapped for the parent's.
func (u *hierarchyUsecase) resolveInherited(
	ctx context.Context,
	in CreateChildInput,
) (CreateChildInput, bool, error) {
	if in.RepoID != "" || in.RepoPath != "" || in.ParentID == "" {
		return in, ownWorktreeOrDefault(in, in.RepoPath != ""), nil
	}
	parent, err := u.workspaces.Get(ctx, in.ParentID)
	if err != nil {
		return in, false, fmt.Errorf("create child: get parent: %w", err)
	}
	in.RepoID = parent.RepoID
	in.ProjectID = parent.ProjectID
	in.ParentBranch = parent.Branch
	in.parentWorktree = parent.WorktreePath
	repo, err := u.repos.FindByKey(ctx, parent.RepoID)
	if err != nil {
		return in, false, fmt.Errorf("create child: get parent repo: %w", err)
	}
	if repo != nil {
		in.RepoPath = repo.Path
		in.RemoteURL = repo.RemoteURL
	}
	return in, ownWorktreeOrDefault(in, parent.Provisioning.HasWorktree()), nil
}

func ownWorktreeOrDefault(
	in CreateChildInput,
	defaultValue bool,
) bool {
	if in.OwnWorktree != nil {
		return *in.OwnWorktree
	}
	return defaultValue
}

// branchWorkspaceExists reports whether a non-deleted workspace already holds
// this branch in the repo. A branch can be checked out in at most one worktree,
// so the repo keeps at most one workspace per branch.
func (u *hierarchyUsecase) branchWorkspaceExists(
	ctx context.Context,
	repoID string,
	branch string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, fmt.Errorf("create child: list workspaces: %w", err)
	}
	for _, w := range all {
		// Only managed workspaces count: the default workspace is the user's own repo folder, whose branch
		// Crowbar does not own.
		if w.RepoID == repoID &&
			w.Branch == branch &&
			!w.IsDefault &&
			w.Status != domain.WorkspaceStatusDeleted {
			return true, nil
		}
	}
	return false, nil
}

// reattachMain re-checks-out branch in the main folder to undo a detach. Best-effort.
func (u *hierarchyUsecase) reattachMain(
	ctx context.Context,
	detached bool,
	repoPath string,
	branch string,
) {
	if !detached {
		return
	}
	if err := u.git.CheckoutBranch(ctx, repoPath, branch); err != nil {
		slog.WarnContext(ctx, "create child: re-attach main worktree after rollback",
			"repo", repoPath, "branch", branch, "err", err)
	}
}

func (u *hierarchyUsecase) resolveLocked(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	protected, err := u.provider.ProtectedBranches(ctx, repoPath)
	if err != nil {
		return false, err
	}
	for _, b := range protected {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}
