package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errGone = errors.New("workspace gone")

// workspaceTable answers the three reads the reader makes from a fixed set of rows.
type workspaceTable struct {
	workspace.Usecase
	rows []domain.Workspace
}

func (t workspaceTable) Get(_ context.Context, id string) (domain.Workspace, error) {
	for _, ws := range t.rows {
		if ws.ID == id {
			return ws, nil
		}
	}
	return domain.Workspace{}, errGone
}

func (t workspaceTable) List(context.Context) ([]domain.Workspace, error) { return t.rows, nil }

func (t workspaceTable) ListInRepo(_ context.Context, projectID, repoID string) ([]domain.Workspace, error) {
	var out []domain.Workspace
	for _, ws := range t.rows {
		if ws.ProjectID == projectID && ws.RepoID == repoID {
			out = append(out, ws)
		}
	}
	return out, nil
}

type repoTable struct {
	store.ScopedStore[domain.Repository, string]
	rows []domain.Repository
}

func (t repoTable) FindByKey(_ context.Context, id string) (*domain.Repository, error) {
	for _, r := range t.rows {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, nil
}

func (t repoTable) FindWhere(_ context.Context, match domain.Repository) ([]domain.Repository, error) {
	var out []domain.Repository
	for _, r := range t.rows {
		if r.ProjectID == match.ProjectID {
			out = append(out, r)
		}
	}
	return out, nil
}

func statusReader() workspaceGitStatusReader {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return workspaceGitStatusReader{
		workspace: workspaceTable{rows: []domain.Workspace{
			{ID: "home", ProjectID: "p1", Kind: domain.WorkspaceKindHome},
			{ID: "main", ProjectID: "p1", RepoID: "r1", IsDefault: true, ParentID: "home", Provisioning: domain.WorkspaceProvisioned},
			{ID: "feat", ProjectID: "p1", RepoID: "r1", Status: domain.WorkspaceStatusLocked, ParentID: "other", CreatedAt: created},
			{ID: "fork", ProjectID: "p1", RepoID: "r1", ParentID: "feat", Provisioning: domain.WorkspaceShared},
			{ID: "orphan", ProjectID: "p1", RepoID: "r1", ParentID: "main"},
			{ID: "other", ProjectID: "p1", RepoID: "r1"},
			{ID: "dangling", ProjectID: "p1", RepoID: "r1", ParentID: "deleted-long-ago"},
			{ID: "stranger", ProjectID: "p2", RepoID: "r2"},
			{ID: "p3home", ProjectID: "p3", Kind: domain.WorkspaceKindHome, Status: domain.WorkspaceStatusDeleted},
		}},
		repos: repoTable{rows: []domain.Repository{
			{ID: "r1", ProjectID: "p1"}, {ID: "r1b", ProjectID: "p1"}, {ID: "r2", ProjectID: "p2"}, {ID: "r3", ProjectID: "p3"},
		}},
	}
}

func TestWorkspaceGitStatusReader_ReadsEachFactFromTheReadModel(t *testing.T) {
	r, ctx := statusReader(), context.Background()

	repo, err := r.RepoOf(ctx, "feat")
	require.NoError(t, err)
	assert.Equal(t, "r1", repo)
	_, err = r.RepoOf(ctx, "missing")
	require.ErrorIs(t, err, errGone)

	createdAt, err := r.CreatedAtOf(ctx, "feat")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), createdAt)
	_, err = r.CreatedAtOf(ctx, "missing")
	require.ErrorIs(t, err, errGone)

	branch, err := r.RendersAsBranch(ctx, "feat")
	require.NoError(t, err)
	assert.True(t, branch)
	branch, err = r.RendersAsBranch(ctx, "fork")
	require.NoError(t, err)
	assert.False(t, branch)
	_, err = r.RendersAsBranch(ctx, "missing")
	require.ErrorIs(t, err, errGone)

	for id, want := range map[string]bool{"feat": true, "missing": false} {
		exists, err := r.Exists(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, exists, id)
	}
}

func TestWorkspaceGitStatusReader_OnlyAHomeOrADefaultCheckoutOutlivesItsChats(t *testing.T) {
	r, ctx := statusReader(), context.Background()

	for id, want := range map[string]bool{"home": true, "main": true, "feat": false} {
		got, err := r.OutlivesItsChats(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id)
	}
	_, err := r.OutlivesItsChats(ctx, "missing")
	require.ErrorIs(t, err, errGone)
}

func TestWorkspaceGitStatusReader_ProvisionedOnlyEverRefuses(t *testing.T) {
	r, ctx := statusReader(), context.Background()

	for id, want := range map[string]bool{"main": true, "fork": true, "orphan": false, "missing": true} {
		got, err := r.Provisioned(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id)
	}
}

func TestWorkspaceGitStatusReader_RepoIDsForHomeStaysInsideTheHomesProject(t *testing.T) {
	r, ctx := statusReader(), context.Background()

	ids, err := r.RepoIDsForHome(ctx, "home")

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"r1": true, "r1b": true}, ids)
	_, err = r.RepoIDsForHome(ctx, "missing")
	require.ErrorIs(t, err, errGone)
}

func TestWorkspaceGitStatusReader_HomeOfRepoSkipsADeletedHome(t *testing.T) {
	r, ctx := statusReader(), context.Background()

	for repo, want := range map[string]string{"r1": "home", "r3": "", "unknown": ""} {
		got, err := r.HomeOfRepo(ctx, repo)
		require.NoError(t, err)
		assert.Equal(t, want, got, repo)
	}
}

func TestWorkspaceGitStatusReader_ARepoListsItsLockedBranchesAndItsDefaultCheckout(t *testing.T) {
	r, ctx := statusReader(), context.Background()

	branches, err := r.BranchRowsOf(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, []string{"feat"}, branches)
	none, err := r.BranchRowsOf(ctx, "unknown")
	require.NoError(t, err)
	assert.Empty(t, none)

	def, err := r.DefaultWorkspaceOf(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, "main", def)
	def, err = r.DefaultWorkspaceOf(ctx, "r1b")
	require.NoError(t, err)
	assert.Empty(t, def, "a repo with no default checkout has none")
	def, err = r.DefaultWorkspaceOf(ctx, "unknown")
	require.NoError(t, err)
	assert.Empty(t, def)
}

func TestWorkspaceGitStatusReader_AForkParentThatDrawsNoRowReducesToNone(t *testing.T) {
	r, ctx := statusReader(), context.Background()

	for id, want := range map[string]string{
		"fork":     "feat",  // a visible branch parent
		"orphan":   "",      // its parent is the default checkout
		"main":     "",      // its parent is a project home
		"feat":     "other", // an ordinary workspace parent draws its own row
		"dangling": "",      // its parent no longer resolves
		"home":     "",      // no parent at all
	} {
		got, err := r.VisibleForkParent(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, got, id)
	}
	_, err := r.VisibleForkParent(ctx, "missing")
	require.ErrorIs(t, err, errGone)
}
