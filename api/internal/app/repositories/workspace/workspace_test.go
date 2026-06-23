package workspace_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func wsAsynxFactory(
	es asynxModels.Store,
) (asynx.Asynx[domain.Workspace], error) {
	return asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func newAdapter(
	t *testing.T,
	home string,
) *adapter.Container {
	t.Helper()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func newRepo(
	t *testing.T,
) (context.Context, workspace.Workspace) {
	t.Helper()
	repo, err := workspace.New(newAdapter(t, t.TempDir()), func(context.Context, domain.Workspace) {}, wsAsynxFactory)
	require.NoError(t, err)
	return context.Background(), repo
}

func newRepoWithBroadcast(
	t *testing.T,
	broadcast func(context.Context, domain.Workspace),
) (context.Context, workspace.Workspace) {
	t.Helper()
	repo, err := workspace.New(newAdapter(t, t.TempDir()), broadcast, wsAsynxFactory)
	require.NoError(t, err)
	return context.Background(), repo
}

func TestWorkspace_SetLastError_SetsAndClears(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	got, err := repo.SetLastError(ctx, "w1", "boom")
	require.NoError(t, err)
	assert.Equal(t, "boom", got.LastError)

	reloaded, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "boom", reloaded.LastError)

	// A successful mutating command clears the stale error.
	cleared, err := repo.SyncWorkingTreeState(ctx, workspace.SyncInput{ID: "w1"}, now)
	require.NoError(t, err)
	assert.Empty(t, cleared.LastError)
}

func TestWorkspace_SetLastError_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.SetLastError(ctx, "no-such", "x")
	assert.Error(t, err)
}

func TestWorkspace_Delete_BroadcastsDeletedTombstone(t *testing.T) {
	var got []domain.Workspace
	broadcast := func(_ context.Context, ws domain.Workspace) { got = append(got, ws) }
	ctx, repo := newRepoWithBroadcast(t, broadcast)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, "w1"))

	var tombstone domain.Workspace
	var found bool
	for _, ws := range got {
		if ws.ID == "w1" && ws.Status == domain.WorkspaceStatusDeleted {
			tombstone = ws
			found = true
		}
	}
	require.True(t, found, "Delete must broadcast a deleted-status row")
	assert.Equal(t, "p1", tombstone.ProjectID)
	assert.Equal(t, "r1", tombstone.RepoID)
}

func TestWorkspace_Create_RoundTrips(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()

	created, err := repo.Create(ctx, workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "feature/x",
	}, now)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusNew, created.Status)

	reloaded, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "feature/x", reloaded.Branch)
	assert.Equal(t, domain.WorkspaceStatusNew, reloaded.Status)
	assert.Equal(t, "p1", reloaded.ProjectID)
}

func TestCreate_WritesLocationIndexAndPerEntityStores(t *testing.T) {
	home := t.TempDir()
	repo, err := workspace.New(newAdapter(t, home), func(context.Context, domain.Workspace) {}, wsAsynxFactory)
	require.NoError(t, err)
	ctx := context.Background()

	_, err = repo.Create(ctx, workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	storages := filepath.Join(home, "projects", "p1", "r1", "workspaces", "w1", "storages")
	_, esErr := os.Stat(filepath.Join(storages, "event_stream.db"))
	assert.NoError(t, esErr, "per-entity event_stream.db must exist")
	_, viewErr := os.Stat(filepath.Join(storages, "view.db"))
	assert.NoError(t, viewErr, "per-entity view.db must exist")
}

func TestGet_ResolvesViaIndex(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "feature/x",
	}, now)
	require.NoError(t, err)

	got, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "feature/x", got.Branch)
}

func TestList_AcrossEntities(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "w2", RepoID: "r2", ProjectID: "p1"}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "w3", RepoID: "r1", ProjectID: "p2"}, now)
	require.NoError(t, err)

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestDelete_RemovesIndexRow(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, "w1"))

	_, err = repo.Get(ctx, "w1")
	assert.Error(t, err, "Get must fail once the location row is gone")

	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestPersistence_AcrossReopen(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()
	now := time.Unix(1000, 0).UTC()

	first := newAdapterClosable(t, home)
	repo1, err := workspace.New(first, func(context.Context, domain.Workspace) {}, wsAsynxFactory)
	require.NoError(t, err)
	_, err = repo1.Create(ctx, workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "persisted",
	}, now)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second := newAdapter(t, home)
	repo2, err := workspace.New(second, func(context.Context, domain.Workspace) {}, wsAsynxFactory)
	require.NoError(t, err)

	got, err := repo2.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "persisted", got.Branch)

	all, err := repo2.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func newAdapterClosable(
	t *testing.T,
	home string,
) *adapter.Container {
	t.Helper()
	c, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	return c
}

func TestWorkspace_SyncKeepsNewStatus(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	synced, err := repo.SyncWorkingTreeState(ctx, workspace.SyncInput{
		ID:         "w1",
		Added:      10,
		Deleted:    2,
		HasCommits: true,
	}, now)
	require.NoError(t, err)
	// Dual-write (W4-mig-1): the new→"" transition is removed; status stays "new".
	assert.Equal(t, domain.WorkspaceStatusNew, synced.Status)
	assert.Equal(t, 10, synced.Added)

	reloaded, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusNew, reloaded.Status)
}

func TestWorkspace_Create_RoundTrips_Timestamps(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()

	_, err := repo.Create(ctx, workspace.CreateInput{
		ID:        "w2",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "feature/ts",
	}, now)
	require.NoError(t, err)

	reloaded, err := repo.Get(ctx, "w2")
	require.NoError(t, err)
	assert.Equal(t, now, reloaded.CreatedAt)
	assert.Equal(t, now, reloaded.LastActivity)
}

func TestWorkspace_Create_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	in := workspace.CreateInput{ID: "w3", RepoID: "r1", ProjectID: "p1"}

	_, err := repo.Create(ctx, in, now)
	require.NoError(t, err)

	_, err = repo.Create(ctx, in, now)
	assert.Error(t, err)
}

func TestWorkspace_Get_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Get(ctx, "does-not-exist")
	assert.Error(t, err)
}

func TestWorkspace_Sync_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.SyncWorkingTreeState(ctx, workspace.SyncInput{ID: "does-not-exist"}, now)
	assert.Error(t, err)
}

func TestWorkspace_SyncProviderState_SetsPR(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	got, err := repo.SyncProviderState(ctx, workspace.ProviderInput{
		ID:        "w1",
		Protected: true,
		HasPR:     true,
		PRStatus:  "open",
		PRUrl:     "u",
	}, now)
	require.NoError(t, err)
	// Protected wins per D4 precedence → Status=locked.
	assert.Equal(t, domain.WorkspaceStatusLocked, got.Status)
	assert.Equal(t, "u", got.PRUrl)
}

func TestWorkspace_SetMergeStrategy(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	got, err := repo.SetMergeStrategy(ctx, "w1", gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.Equal(t, gitdomain.MergeStrategySquash, got.MergeStrategy)
}

func TestWorkspace_Reparent_TouchActivity_ForkPoint(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	_, err = repo.TouchActivity(ctx, "w1", now)
	require.NoError(t, err)
	rp, err := repo.Reparent(ctx, "w1", "p2", "sha2", now)
	require.NoError(t, err)
	assert.Equal(t, "p2", rp.ParentID)
	fp, err := repo.UpdateForkPoint(ctx, "w1", "sha3")
	require.NoError(t, err)
	assert.Equal(t, "sha3", fp.ForkPointSha)
}

func TestWorkspace_Delete_Forgets(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, "w1"))
	_, err = repo.Get(ctx, "w1")
	assert.Error(t, err)
}

func TestWorkspace_SyncProviderState_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.SyncProviderState(ctx, workspace.ProviderInput{ID: "no-such"}, now)
	assert.Error(t, err)
}

func TestWorkspace_SetMergeStrategy_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.SetMergeStrategy(ctx, "no-such", gitdomain.MergeStrategyMerge)
	assert.Error(t, err)
}

func TestWorkspace_TouchActivity_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.TouchActivity(ctx, "no-such", now)
	assert.Error(t, err)
}

func TestWorkspace_Reparent_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Reparent(ctx, "no-such", "p", "sha", now)
	assert.Error(t, err)
}

func TestWorkspace_UpdateForkPoint_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.UpdateForkPoint(ctx, "no-such", "sha")
	assert.Error(t, err)
}

func TestWorkspace_Delete_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	err := repo.Delete(ctx, "no-such")
	assert.Error(t, err)
}

func TestWorkspace_SetParentFromPR(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	got, err := repo.SetParentFromPR(ctx, "w1", "parent")
	require.NoError(t, err)
	assert.Equal(t, "parent", got.ParentID)
}

func TestWorkspace_SetParentFromPR_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.SetParentFromPR(ctx, "no-such", "p")
	assert.Error(t, err)
}

func TestWorkspace_List(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "w2", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestCreate_PersistsIsDefault(t *testing.T) {
	ctx, repo := newRepo(t)

	created, err := repo.Create(ctx, workspace.CreateInput{
		ID:        "w-default",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "develop",
		IsDefault: true,
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	assert.True(t, created.IsDefault, "Create must return IsDefault")

	got, err := repo.Get(ctx, "w-default")
	require.NoError(t, err)
	assert.True(t, got.IsDefault, "Get must round-trip IsDefault")
}

func TestWorkspace_New_NilGuards(t *testing.T) {
	_, err := workspace.New(nil, func(context.Context, domain.Workspace) {}, wsAsynxFactory)
	assert.Error(t, err)

	_, err = workspace.New(newAdapter(t, t.TempDir()), func(context.Context, domain.Workspace) {}, nil)
	assert.Error(t, err)
}

// TestWorkspace_EntityCacheStaysBounded proves the per-entity LRU is actually
// bounded: before the release-after-use fix, every entityFor/entityForLocation
// discarded the registry release func, pinning every wsEntity (each ~ an asynx
// instance + its ES/View SQLite handles) for the process lifetime, so the cache
// grew without bound. Now each repo op acquires then releases, so touching far
// more than the configured bound of distinct workspaces leaves the registry
// holding at most `bound` entries.
func TestWorkspace_EntityCacheStaysBounded(t *testing.T) {
	const bound = 8
	const n = 70 // > bound; also > the default 64 so this fails on the old code path
	repo, err := workspace.New(
		newAdapter(t, t.TempDir()),
		func(context.Context, domain.Workspace) {},
		wsAsynxFactory,
		workspace.WithMaxOpenEntities(bound),
	)
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Unix(1000, 0).UTC()

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("w%d", i)
		_, createErr := repo.Create(ctx, workspace.CreateInput{ID: id, RepoID: "r1", ProjectID: "p1"}, now)
		require.NoError(t, createErr)
		// A second op on the same entity exercises the cache-hit acquire/release
		// path too, not just the build-on-miss path.
		_, getErr := repo.Get(ctx, id)
		require.NoError(t, getErr)
	}

	// Every op above released its handle, so the LRU has evicted down to the bound.
	// The whole point of the fix: cached entities never exceed maxOpen.
	cached := workspace.CachedEntityCount(repo)
	t.Logf("cached entities after touching %d distinct workspaces (bound=%d): %d", n, bound, cached)
	require.GreaterOrEqual(t, cached, 0, "test accessor must reach the concrete repo")
	assert.LessOrEqualf(t, cached, bound,
		"entity cache must stay bounded by maxOpen=%d after touching %d distinct workspaces, got %d cached",
		bound, n, cached)

	// And a list across all N still works (re-opening evicted entities lazily),
	// proving eviction did not lose any data.
	all, listErr := repo.List(ctx)
	require.NoError(t, listErr)
	assert.Len(t, all, n)
}

func TestWorkspace_Create_AsynxFactoryError(t *testing.T) {
	sentinel := errors.New("boom")
	repo, err := workspace.New(
		newAdapter(t, t.TempDir()),
		func(context.Context, domain.Workspace) {},
		func(asynxModels.Store) (asynx.Asynx[domain.Workspace], error) {
			return nil, sentinel
		},
	)
	require.NoError(t, err)

	_, err = repo.Create(context.Background(), workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
	}, time.Unix(1, 0).UTC())
	assert.ErrorIs(t, err, sentinel)
}
