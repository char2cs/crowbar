package hierarchy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// DeleteRepoWorkspaces exists so a repo delete can broadcast its tombstone the
// moment the ROW is gone and tear the workspaces down behind it. That only works
// if the cascade no longer needs the row — so it takes the repo path from the
// caller, and must still run the git teardown with it.
func TestDeleteRepoWorkspaces_UsesTheCallersPathWhenTheRepoRowIsGone(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "alpha", WorktreePath: "/tmp/crowbar-test/projects/p/app/a/worktree", Provisioning: domain.WorkspaceProvisioned},
		{ID: "w2", RepoID: "r2", ProjectID: "p1", Branch: "other", WorktreePath: "/tmp/crowbar-test/projects/p/app/b/worktree", Provisioning: domain.WorkspaceProvisioned},
	}
	deleted := []string{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			for _, w := range all {
				if w.ID == id {
					return w, nil
				}
			}
			return domain.Workspace{}, problem.ErrNotFound
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	g := &fakeGit{}
	// The repo store answers NOTHING: the row is already deleted, which is the
	// whole situation this method is for.
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.NoError(t, err)

	assert.Equal(t, []string{"w1"}, deleted, "only the repo's own workspaces are removed")
	assert.Contains(t, g.ops(), "WorktreeRemove",
		"the git teardown must still run — skipping it strands a live worktree "+
			"registration in the user's own repository")
	for _, c := range g.calls {
		if c.op == "WorktreeRemove" {
			assert.Equal(t, "/repo", c.args[0],
				"and it must use the path the caller handed over")
		}
	}
}

// A workspace must still be removable when the alias cannot be worked out — the
// alias is derived state, and a broken link is litter the boot sweep clears, not
// a reason to strand a workspace.
func TestDeleteRepoWorkspaces_RemovesEvenWhenTheAliasCannotBeResolved(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "alpha", WorktreePath: "/tmp/crowbar-test/projects/p/app/a/worktree", Provisioning: domain.WorkspaceProvisioned},
	}
	deleted := []string{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return all[0], nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	// The repo store fails outright: no slug, so no alias can be named.
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{},
		&fakeRepoStore{err: assert.AnError}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.NoError(t, err)
	assert.Equal(t, []string{"w1"}, deleted)
}

// A workspace with no branch has no alias to withdraw, and a placeholder has no
// worktree to remove — both still lose their row.
func TestDeleteRepoWorkspaces_HandlesPlaceholdersAndBranchlessRows(t *testing.T) {
	all := []domain.Workspace{
		{ID: "w-placeholder", RepoID: "r1", ProjectID: "p1", Branch: "held", Provisioning: domain.WorkspacePlaceholder},
		{ID: "w-branchless", RepoID: "r1", ProjectID: "p1", WorktreePath: "/tmp/crowbar-test/projects/p/app/b/worktree", Provisioning: domain.WorkspaceProvisioned},
	}
	deleted := []string{}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return all, nil },
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			for _, w := range all {
				if w.ID == id {
					return w, nil
				}
			}
			return domain.Workspace{}, problem.ErrNotFound
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{missing: true}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"w-placeholder", "w-branchless"}, deleted)
	// The placeholder contributes no git at all: it has no worktree of its own,
	// and its branch is held by something else that must not be touched.
	assert.Equal(t, 1, countOp(g.ops(), "WorktreeRemove"),
		"only the row that actually has a worktree reaches git")
}

// A listing failure is the one thing that stops the cascade: without the list
// there is nothing to walk, and reporting success would claim work never done.
func TestDeleteRepoWorkspaces_ReportsAListingFailure(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, assert.AnError },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.Error(t, err)
}
