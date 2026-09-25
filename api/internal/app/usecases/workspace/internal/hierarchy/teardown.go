package hierarchy

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// teardown is what deleting one workspace does to git. It is decided once, by
// teardownOf, so the loss assessment and the removal can never disagree.
type teardown struct {
	// worktree is the checkout to remove, or "" when it is not Crowbar's.
	worktree string
	force    bool
	// dropBranch is the branch to delete, or "" when it stays.
	dropBranch string
	// reattach puts the main folder back on the default branch the workspace held.
	reattach bool
}

// teardownOf decides a workspace's teardown. Only a worktree Crowbar created is
// ever removed, and it is forced only when unlocked and holding no other row's
// files; only a branch Crowbar created, unlocked and not the default, is deleted.
func (u *hierarchyUsecase) teardownOf(
	ws domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) teardown {
	// No repo to run git against, or a placeholder whose branch is held elsewhere.
	if repo.path == "" || ws.Provisioning == domain.WorkspacePlaceholder {
		return teardown{}
	}
	locked := ws.Status == domain.WorkspaceStatusLocked
	var td teardown
	if u.crowbarWorktree(ws, repo.path) {
		td.worktree = ws.WorktreePath
		td.force = !locked && !worktreepath.HoldsAnother(ws.WorktreePath, otherPaths(ws.ID, all))
	}
	switch {
	case ws.Branch == "":
	case ws.Branch == repo.defaultBranch:
		td.reattach = true
	case ws.CreatedBranch && !locked:
		td.dropBranch = ws.Branch
	}
	return td
}

// crowbarWorktree reports whether ws's checkout is one Crowbar made: recorded
// as provisioned, under <home>/projects, and not the repo's own checkout. The
// main working tree and any checkout adopted in place are the user's.
func (u *hierarchyUsecase) crowbarWorktree(
	ws domain.Workspace,
	repoPath string,
) bool {
	path := ws.WorktreePath
	if ws.Provisioning != domain.WorkspaceProvisioned || path == "" ||
		worktreepath.SamePath(path, repoPath) {
		return false
	}
	home, err := u.crowbarHome()
	if err != nil || home == "" {
		return false
	}
	root := filepath.Join(home, "projects")
	return worktreepath.UnderHome(filepath.Clean(path), root) ||
		worktreepath.UnderHome(worktreepath.ResolvePath(path), worktreepath.ResolvePath(root))
}

// workAtRisk measures what td would destroy: uncommitted paths in a worktree it
// force-removes, and commits reachable only from that worktree's HEAD or from
// the branch it deletes. Stashes are shared by the whole repo and survive.
func (u *hierarchyUsecase) workAtRisk(
	ctx context.Context,
	ws domain.Workspace,
	repo repoRef,
	td teardown,
) (domain.WorkAtRisk, error) {
	risk := domain.WorkAtRisk{WorkspaceID: ws.ID, Branch: ws.Branch}
	live := td.worktree != "" && worktreepath.IsLiveCheckout(td.worktree)
	if live && td.force {
		n, err := u.git.UncommittedFiles(ctx, td.worktree)
		if err != nil {
			return risk, fmt.Errorf("work at risk: %s: uncommitted: %w", ws.ID, err)
		}
		risk.UncommittedFiles = n
	}
	dir, tips := repo.path, []string(nil)
	if live {
		dir, tips = td.worktree, append(tips, "HEAD")
	}
	if td.dropBranch != "" {
		tips = append(tips, "refs/heads/"+td.dropBranch)
	}
	if len(tips) == 0 {
		return risk, nil
	}
	n, err := u.git.UnmergedCommits(ctx, dir, tips, td.dropBranch)
	if err != nil {
		return risk, fmt.Errorf("work at risk: %s: unmerged commits: %w", ws.ID, err)
	}
	risk.UnmergedCommits = n
	return risk, nil
}

// refuseLoss is the one rule every delete passes through: without consent, a
// delete that would destroy work existing nowhere else stops before touching
// anything and names that work. An assessment git cannot make also stops it.
func (u *hierarchyUsecase) refuseLoss(
	ctx context.Context,
	consent domain.DeleteConsent,
	doomed []domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) error {
	if consent == domain.DiscardWorkAtRisk {
		return nil
	}
	risks, err := u.assess(ctx, doomed, repo, all)
	if err != nil {
		return err
	}
	if len(risks) > 0 {
		return &domain.WorkAtRiskError{Workspaces: risks}
	}
	return nil
}

// assess lists the work each doomed workspace's teardown would destroy.
func (u *hierarchyUsecase) assess(
	ctx context.Context,
	doomed []domain.Workspace,
	repo repoRef,
	all []domain.Workspace,
) ([]domain.WorkAtRisk, error) {
	var risks []domain.WorkAtRisk
	for _, ws := range doomed {
		risk, err := u.workAtRisk(ctx, ws, repo, u.teardownOf(ws, repo, all))
		if err != nil {
			return nil, err
		}
		if risk.Lost() {
			risks = append(risks, risk)
		}
	}
	return risks, nil
}
