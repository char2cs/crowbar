package main

import "context"

const (
	seedFeatureBranch = "feature/pricing-rounding"
	statusDeleted     = "deleted"
)

type workspaceDTO struct {
	ID        string `json:"id"`
	RepoID    string `json:"repoId"`
	Branch    string `json:"branch"`
	ParentID  string `json:"parentId"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	IsDefault bool   `json:"isDefault"`
	LocalPath string `json:"localPath"`
}

// pickBaseWorkspace finds the locked, Crowbar-managed worktree sitting on the
// repo's default branch — the branch-tree root a feature workspace forks from.
//
// It is deliberately NOT the isDefault "home" row. Import adopts the repo folder
// itself as home and detaches it so the base branch is free for its own managed
// worktree; home is the unmanaged checkout Crowbar never runs git on, and a
// child forked off it is parented to a row that owns no branch.
func pickBaseWorkspace(
	repoID string,
	defaultBranch string,
) func([]workspaceDTO) (workspaceDTO, bool) {
	return func(list []workspaceDTO) (workspaceDTO, bool) {
		for _, w := range list {
			if w.RepoID == repoID && w.Branch == defaultBranch && !w.IsDefault && w.Status != statusDeleted {
				return w, true
			}
		}
		return workspaceDTO{}, false
	}
}

// pickFeatureWorkspace requires a populated LocalPath: the workspace row is what
// the seed then commits into, and a row without a worktree on disk is not yet
// usable for that.
func pickFeatureWorkspace(
	repoID string,
) func([]workspaceDTO) (workspaceDTO, bool) {
	return func(list []workspaceDTO) (workspaceDTO, bool) {
		for _, w := range list {
			if w.RepoID == repoID && w.Branch == seedFeatureBranch &&
				w.Status != statusDeleted && w.LocalPath != "" {
				return w, true
			}
		}
		return workspaceDTO{}, false
	}
}

func workspacesPath(
	projectID string,
	repoID string,
) string {
	return repoScopePath(projectID, repoID, "/workspaces")
}

// ensureWorkspace creates the seed's feature workspace under the repo's base
// branch, or reuses the one already there.
func ensureWorkspace(
	ctx context.Context,
	d *daemon,
	repo repoDTO,
	parentID string,
) (workspaceDTO, bool, error) {
	path := workspacesPath(repo.ProjectID, repo.ID)
	pick := pickFeatureWorkspace(repo.ID)
	existing, err := getData[[]workspaceDTO](ctx, d, "list workspaces", path)
	if err != nil {
		return workspaceDTO{}, false, err
	}
	if found, ok := pick(existing); ok {
		return found, false, nil
	}
	body := map[string]any{"branch": seedFeatureBranch, "parentId": parentID}
	if err := d.postAccepted(ctx, "create workspace", path, body); err != nil {
		return workspaceDTO{}, false, err
	}
	created, err := waitFor(ctx, d, "the "+seedFeatureBranch+" workspace", path, pick)
	return created, true, err
}

// waitForBaseWorkspace blocks until repo import has provisioned the locked base
// worktree. It is provisioned after the repo row is broadcast, so a workspace
// create issued the instant the repo appears would have no parent to fork from.
func waitForBaseWorkspace(
	ctx context.Context,
	d *daemon,
	repo repoDTO,
) (workspaceDTO, error) {
	return waitFor(
		ctx, d,
		"the locked "+repo.DefaultBranch+" workspace",
		workspacesPath(repo.ProjectID, repo.ID),
		pickBaseWorkspace(repo.ID, repo.DefaultBranch),
	)
}
