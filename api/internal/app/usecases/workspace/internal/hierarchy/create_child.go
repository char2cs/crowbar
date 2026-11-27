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

type CreateChildInput struct {
	RepoID         string
	ProjectID      string
	RepoPath       string
	RemoteURL      string
	Branch         string
	ParentID       string
	ParentBranch   string
	OwnWorktree    *bool
	ForceLocked    bool
	FromParentHead bool
	parentWorktree string
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
	if in.RepoPath == "" || !ownWorktree {
		return u.createDirectRow(ctx, in)
	}
	if in.Branch == "" {
		branch, genErr := u.generateBranchName(ctx, in.RepoPath)
		if genErr != nil {
			return domain.Workspace{}, fmt.Errorf("create child: %w", genErr)
		}
		in.Branch = branch
	}
	exists, err := u.branchWorkspaceExists(ctx, in.RepoID, in.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	if exists {
		return domain.Workspace{}, fmt.Errorf("%w (repo %s, branch %q)", ErrBranchWorkspaceExists, in.RepoID, in.Branch)
	}
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
		u.cleanupFailedWorktree(ctx, in.RepoPath, path, in.Branch, detached, added.createdBranch)
		return domain.Workspace{}, err
	}
	if nErr := u.mintWorkspaceNode(ctx, ws.ID); nErr != nil {
		u.cleanupFailedWorktree(ctx, in.RepoPath, path, in.Branch, detached, added.createdBranch)
		u.discardWorkspaceRow(ctx, ws.ID, "create child")
		return domain.Workspace{}, nErr
	}
	u.clearDetachedHomeBranch(ctx, detached, in.RepoID, in.Branch)
	return ws, nil
}

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

func (u *hierarchyUsecase) createDirectRow(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		ParentID:     in.ParentID,
		Protected:    in.ForceLocked,
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

func (u *hierarchyUsecase) cleanupFailedWorktree(
	ctx context.Context,
	repoPath string,
	path string,
	branch string,
	detached bool,
	createdBranch bool,
) {
	ctx = context.WithoutCancel(ctx)
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
		if w.RepoID == repoID &&
			w.Branch == branch &&
			!w.IsDefault &&
			w.Status != domain.WorkspaceStatusDeleted {
			return true, nil
		}
	}
	return false, nil
}

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
