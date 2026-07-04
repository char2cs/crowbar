package directory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/directory"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newDirectory(
	t *testing.T,
) directory.Directory {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	d, err := directory.New(db)
	require.NoError(t, err)
	return d
}

func TestDirectory_UpsertAndListByRepo(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	ws := domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main"}

	require.NoError(t, d.Upsert(ctx, ws))

	rows, err := d.ListByRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
	assert.Equal(t, "main", rows[0].Branch)
}

func TestDirectory_ListByRepo_IsolatesOtherRepos(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w2", ProjectID: "p1", RepoID: "r2"}))

	rows, err := d.ListByRepo(ctx, "p1", "r1")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
}

func TestDirectory_ListByRepo_EmptyRepoID_MatchesWholeProject(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w2", ProjectID: "p1", RepoID: "r2"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w3", ProjectID: "p2", RepoID: "r3"}))

	rows, err := d.ListByRepo(ctx, "p1", "")

	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestDirectory_ListByRepo_BlankScope_MatchesEverything(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w2", ProjectID: "p2", RepoID: "r2"}))

	rows, err := d.ListByRepo(ctx, "", "")

	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestDirectory_Upsert_OverwritesExistingRow(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "renamed"}))

	rows, err := d.ListByRepo(ctx, "p1", "r1")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "renamed", rows[0].Branch)
}

func TestDirectory_Delete_RemovesRow(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))

	require.NoError(t, d.Delete(ctx, "w1"))

	rows, err := d.ListByRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestDirectory_Delete_UnknownID_NoOp(t *testing.T) {
	d := newDirectory(t)
	require.NoError(t, d.Delete(context.Background(), "missing"))
}

func TestDirectory_Rebuild_ReplacesEntireTable(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "stale", ProjectID: "p1", RepoID: "r1"}))

	require.NoError(t, d.Rebuild(ctx, []domain.Workspace{
		{ID: "w1", ProjectID: "p1", RepoID: "r1"},
		{ID: "w2", ProjectID: "p1", RepoID: "r1"},
	}))

	rows, err := d.ListByRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	ids := []string{rows[0].ID, rows[1].ID}
	assert.ElementsMatch(t, []string{"w1", "w2"}, ids)
	assert.NotContains(t, ids, "stale")
}

func TestDirectory_Rebuild_EmptyInput_ClearsTable(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))

	require.NoError(t, d.Rebuild(ctx, nil))

	rows, err := d.ListByRepo(ctx, "", "")
	require.NoError(t, err)
	assert.Empty(t, rows)
}
