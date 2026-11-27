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
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{},
		&fakeRepoStore{err: assert.AnError}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.NoError(t, err)
	assert.Equal(t, []string{"w1"}, deleted)
}

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
	assert.Equal(t, 1, countOp(g.ops(), "WorktreeRemove"),
		"only the row that actually has a worktree reaches git")
}

func TestDeleteRepoWorkspaces_ReportsAListingFailure(t *testing.T) {
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, assert.AnError },
	}
	uc := hierarchy.New(ws, &fakeGit{}, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.Error(t, err)
}
