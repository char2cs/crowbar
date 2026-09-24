package workspace_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/drain"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

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

// wsAx builds a real workspace asynx over the adapter's singleton per-type event
// store, shutting it down before the adapter closes (the asynx sits on the ES
// handle the adapter owns).
func wsAx(
	t *testing.T,
	ad *adapter.Container,
) asynx.Asynx[domain.Workspace] {
	t.Helper()
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(ad.WorkspaceES()).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func buildRepo(
	t *testing.T,
	ad *adapter.Container,
) workspace.Workspace {
	t.Helper()
	repo, err := workspace.New(wsAx(t, ad), ad.WorkspaceES(), ad.WorkspaceView())
	require.NoError(t, err)
	return repo
}

func newRepo(
	t *testing.T,
) (context.Context, workspace.Workspace) {
	t.Helper()
	repo := buildRepo(t, newAdapter(t, t.TempDir()))
	return context.Background(), repo
}

// listQuiescent drains the async store projection (Send returns before the read
// model is updated — decision 4), then reads List and asserts it holds want rows —
// deterministically, with no polling and no timeout.
func listQuiescent(
	t *testing.T,
	ctx context.Context,
	repo workspace.Workspace,
	want int,
) []domain.Workspace {
	t.Helper()
	workspace.WaitQuiescentForTest(repo)
	rows, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, rows, want)
	return rows
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

// TestRegression_RenameBranch_LeavesTheWorktreePathUntouched pins that a rename
// never moves the workspace: the directory is fixed at creation and never tracks
// the branch. The purge removes exactly the tombstone's WorktreePath, so a
// rename that repointed it would aim a later delete at whatever now occupies
// the other name.
func TestRegression_RenameBranch_LeavesTheWorktreePathUntouched(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo := buildRepo(t, ad)
	ctx := context.Background()
	const path = "/h/projects/p1/github.com/o/r/a/worktree"

	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "a", WorktreePath: path,
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	renamed, err := repo.RenameBranch(ctx, "w1", "b")
	require.NoError(t, err)
	assert.Equal(t, "b", renamed.Branch, "the branch must move")
	assert.Equal(t, path, renamed.WorktreePath, "the worktree path must not")
}

func TestGet_FoldsFromLog(t *testing.T) {
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

func TestList_AcrossAggregates(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "w2", RepoID: "r2", ProjectID: "p1"}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "w3", RepoID: "r1", ProjectID: "p2"}, now)
	require.NoError(t, err)

	listQuiescent(t, ctx, repo, 3)
}

// TestDelete_PersistsDeletedTombstone proves the Task 7 delete lifecycle: Delete
// is a pure Send that folds Status=deleted; the store projection PERSISTS that
// tombstone (it does NOT Forget synchronously — that is the Task 8 reactor's
// job), so the aggregate still folds from the log and the read-model row survives
// with Status=deleted for the boot orphan-sweep to find (spec §3.8).
func TestDelete_PersistsDeletedTombstone(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, "w1"))

	// The aggregate is tombstoned, not forgotten: Get still folds it.
	got, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusDeleted, got.Status)

	// The read model persists the deleted row (no reactor forgets it in Task 7).
	rows := listQuiescent(t, ctx, repo, 1)
	assert.Equal(t, domain.WorkspaceStatusDeleted, rows[0].Status)
}

func TestPersistence_AcrossReopen(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()
	now := time.Unix(1000, 0).UTC()

	first, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	ax1, err := asynx.New[domain.Workspace]().
		WithEventStore(first.WorkspaceES()).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	repo1, err := workspace.New(ax1, first.WorkspaceES(), first.WorkspaceView())
	require.NoError(t, err)

	_, err = repo1.Create(ctx, workspace.CreateInput{
		ID:        "w1",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "persisted",
	}, now)
	require.NoError(t, err)
	// Ensure the projection persisted the row before we tear the first env down.
	listQuiescent(t, ctx, repo1, 1)

	require.NoError(t, ax1.Shutdown(ctx)) // drain projections, release ES handle
	require.NoError(t, first.Close())     // WAL checkpoint + close all DBs

	second := newAdapter(t, home)
	repo2 := buildRepo(t, second)

	got, err := repo2.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "persisted", got.Branch)

	// The durable store read model survives the restart with ZERO replay.
	all, err := repo2.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
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

func TestWorkspace_SetLock_OverridesTheProviderDecision(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: "/some/path",
	}, now)
	require.NoError(t, err)

	locked := true
	ws, err := repo.SetLock(ctx, "w1", &locked, false)

	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Status)
}

func TestWorkspace_SetLock_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	locked := true
	_, err := repo.SetLock(ctx, "no-such", &locked, false)
	assert.Error(t, err)
}

func TestWorkspace_ResolveConflicts_ClearsPRConflictsStatus(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	_, err = repo.SyncWorkingTreeState(ctx, workspace.SyncInput{ID: "w1", HasConflicts: true}, now)
	require.NoError(t, err)

	ws, err := repo.ResolveConflicts(ctx, "w1", now)

	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusNew, ws.Status)
}

func TestWorkspace_ResolveConflicts_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.ResolveConflicts(ctx, "no-such", time.Unix(1000, 0).UTC())
	assert.Error(t, err)
}

func TestWorkspace_ProvisionInPlace_AttachesTheWorktree(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", HeldByPath: "/holder",
	}, now)
	require.NoError(t, err)

	ws, err := repo.ProvisionInPlace(ctx, "w1", "/new/worktree", "sha1")

	require.NoError(t, err)
	assert.Equal(t, "/new/worktree", ws.WorktreePath)
	assert.Equal(t, "sha1", ws.ForkPointSha)
	assert.Empty(t, ws.HeldByPath, "provisioning in place must clear the prior holder")
}

func TestWorkspace_ProvisionInPlace_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.ProvisionInPlace(ctx, "no-such", "/path", "sha")
	assert.Error(t, err)
}

func TestWorkspace_ClearBranch_BlanksTheBranch(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "feature-x"}, now)
	require.NoError(t, err)

	ws, err := repo.ClearBranch(ctx, "w1")

	require.NoError(t, err)
	assert.Empty(t, ws.Branch)
}

func TestWorkspace_ClearBranch_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.ClearBranch(ctx, "no-such")
	assert.Error(t, err)
}

func TestWorkspace_RenameBranch_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.RenameBranch(ctx, "no-such", "new-branch-name")
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
	listQuiescent(t, ctx, repo, 2)
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
	ad := newAdapter(t, t.TempDir())

	_, err := workspace.New(nil, ad.WorkspaceES(), ad.WorkspaceView())
	assert.Error(t, err, "nil asynx must error")

	_, err = workspace.New(wsAx(t, ad), nil, ad.WorkspaceView())
	assert.Error(t, err, "nil event store must error")

	_, err = workspace.New(wsAx(t, ad), ad.WorkspaceES(), nil)
	assert.Error(t, err, "nil store db must error")
}

// TestWorkspace_New_ErrorFromUnderlyingStore proves New surfaces a failure from
// store.New itself, distinct from the four nil-dependency guards above: every
// dependency here is non-nil, but the store's read-model DB connection is
// already closed, so New must still fail rather than hand back a repo backed by
// a dead connection.
func TestWorkspace_New_ErrorFromUnderlyingStore(t *testing.T) {
	ad := newAdapter(t, t.TempDir())

	storeDB := ad.WorkspaceView()
	sqlDB, err := storeDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = workspace.New(wsAx(t, ad), ad.WorkspaceES(), storeDB)
	assert.Error(t, err)
}

func TestListInRepo_ScopesToTheRepoWhateverARowsProjectSays(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{
		ID: "w2", ProjectID: "p1", RepoID: "r2", Branch: "main",
	}, time.Unix(2, 0).UTC())
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{
		ID: "w3", ProjectID: "p2", RepoID: "r1", Branch: "main",
	}, time.Unix(3, 0).UTC())
	require.NoError(t, err)

	workspace.WaitQuiescentForTest(repo)
	rows, err := repo.ListInRepo(ctx, "p1", "r1")

	require.NoError(t, err)
	ids := []string{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	// w3 is r1's row whose own ProjectID a repo move never got to: the repo row
	// owns the project assignment, so r1's rows are all of them (spec §3 P0-3).
	assert.ElementsMatch(t, []string{"w1", "w3"}, ids)
}

func TestListInRepo_NoMatchesReturnsEmpty(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	rows, err := repo.ListInRepo(ctx, "p1", "does-not-exist")

	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestGetHomeForProject_Found(t *testing.T) {
	ctx, repo := newRepo(t)

	projectID := "proj-abc"
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID:           "ws-home-1",
		ProjectID:    projectID,
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/myproject",
	}, time.Now())
	require.NoError(t, err)

	workspace.WaitQuiescentForTest(repo)
	got, err := repo.GetHomeForProject(ctx, projectID)
	require.NoError(t, err)
	require.Equal(t, "ws-home-1", got.ID)
	require.Equal(t, domain.WorkspaceKindHome, got.Kind)
}

func TestGetHomeForProject_NotFound(t *testing.T) {
	_, repo := newRepo(t)
	_, err := repo.GetHomeForProject(context.Background(), "nonexistent-project")
	require.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestWorkspace_List_StorageError(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo := buildRepo(t, ad)
	sqlDB, err := ad.WorkspaceView().DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.List(context.Background())

	assert.Error(t, err)
}

func TestListInRepo_StorageError(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo := buildRepo(t, ad)
	sqlDB, err := ad.WorkspaceView().DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.ListInRepo(context.Background(), "p1", "r1")

	assert.Error(t, err)
}

func TestGetHomeForProject_StorageError(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo := buildRepo(t, ad)
	sqlDB, err := ad.WorkspaceView().DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.GetHomeForProject(context.Background(), "p1")

	assert.Error(t, err)
}

// TestCreateHome_ProvisionsAHomeWorkspaceFindableByProject proves CreateHome's
// happy path end to end: it mints a fresh id, sets Kind=home, and the result is
// durably findable via GetHomeForProject — the whole point of the lazy
// provisioning GetHomeForProject's own ErrNotFound path exists to trigger.
func TestCreateHome_ProvisionsAHomeWorkspaceFindableByProject(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()

	created, err := repo.CreateHome(ctx, "p1", "/projects/p1", now)

	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceKindHome, created.Kind)
	assert.Equal(t, "p1", created.ProjectID)
	assert.Equal(t, "/projects/p1", created.WorktreePath)
	assert.NotEmpty(t, created.ID, "CreateHome must mint a fresh id")

	workspace.WaitQuiescentForTest(repo)
	found, err := repo.GetHomeForProject(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

// TestWorkspace_Sweep_RedrivesThePurgeForEveryResidualDeletedRow proves the
// boot orphan-sweep seam (spec §3.8, §7-D): every row the durable read model
// still carries as Status=deleted — here, tombstones whose reactor a draining
// gate refused, exactly as at a shutdown — is re-purged by the SAME Purger the
// reactor runs, from the tombstone's own WorktreePath. The live workspace is
// left alone.
//
// w1 is the P0-4 case: a placeholder (no path at creation) provisioned in place
// later. The retired id→path index was written only at creation, so the sweep
// used to find no path for it and leak its worktree.
func TestWorkspace_Sweep_RedrivesThePurgeForEveryResidualDeletedRow(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()

	var purged, removed []string
	gate := drain.New()
	gate.Wait(ctx) // draining: the reactor refuses every event, as at shutdown
	registrar, ok := repo.(workspace.DeleteReactorRegistrar)
	require.True(t, ok)
	require.NoError(t, registrar.RegisterDeleteReactor(
		func(_ context.Context, wsID string) error { purged = append(purged, wsID); return nil },
		func(path string) error { removed = append(removed, path); return nil },
		gate,
	))

	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Protected: true}, now)
	require.NoError(t, err)
	_, err = repo.ProvisionInPlace(ctx, "w1", "/h/projects/p1/r/w1/worktree", "sha")
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "w2", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, "w1"))
	listQuiescent(t, ctx, repo, 2)

	sweeper, ok := repo.(workspace.BootSweeper)
	require.True(t, ok, "the concrete repo must satisfy BootSweeper")
	require.NoError(t, sweeper.Sweep(ctx))

	assert.Equal(t, []string{"w1"}, purged, "only the residual deleted row is re-purged")
	assert.Equal(t, []string{"/h/projects/p1/r/w1/worktree"}, removed,
		"the purge removes the tombstone's own (provisioned) worktree path")
	exists, err := repo.Get(ctx, "w1")
	assert.Error(t, err, "the purged aggregate is Forgotten: %+v", exists)
}

// A sweep with no purger registered is a wiring error, never a silent no-op.
func TestWorkspace_Sweep_RefusesWithoutAPurger(t *testing.T) {
	ctx, repo := newRepo(t)
	require.Error(t, repo.(workspace.BootSweeper).Sweep(ctx))
}

// spyReconciler records the ids passed to OnOpen so a test can assert which read
// paths trigger reconcile-on-open.
type spyReconciler struct {
	mu     sync.Mutex
	opened []string
}

func (s *spyReconciler) OnOpen(
	_ context.Context,
	wsID string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opened = append(s.opened, wsID)
}

func (s *spyReconciler) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.opened...)
}

func buildRepoWithReconciler(
	t *testing.T,
	ad *adapter.Container,
	r workspace.ReconcileOnOpener,
) workspace.Workspace {
	t.Helper()
	repo, err := workspace.New(wsAx(t, ad), ad.WorkspaceES(), ad.WorkspaceView(), workspace.WithReconciler(r))
	require.NoError(t, err)
	return repo
}

// TestGet_TriggersReconcileOnOpen proves the §3.8 per-id read path: a Get
// dispatches reconcile-on-open for that id (off the caller's read path).
func TestGet_TriggersReconcileOnOpen(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	spy := &spyReconciler{}
	repo := buildRepoWithReconciler(t, ad, spy)
	ctx := context.Background()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Empty(t, spy.calls(), "Create must not trigger reconcile")

	_, err = repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, []string{"w1"}, spy.calls(), "Get must trigger reconcile-on-open")
}

// TestList_DoesNotTriggerReconcile proves the §3.8 rule that List reads the
// durable read model directly and MUST NOT fan out a per-workspace reconcile
// (which would reintroduce the wake-storm wedge this refactor kills).
func TestList_DoesNotTriggerReconcile(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	spy := &spyReconciler{}
	repo := buildRepoWithReconciler(t, ad, spy)
	ctx := context.Background()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	listQuiescent(t, ctx, repo, 1)

	assert.Empty(t, spy.calls(), "List must never trigger per-workspace reconcile")
}

// A repo moved between projects re-points its workspaces, and must leave the
// on-disk worktree where it is: the path was derived once and is stored
// absolute, so rewriting it here would strand the tree it names.
func TestWorkspace_SetProject_MovesTheRecordNotTheTree(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: "/tmp/tree/worktree",
	}, now)
	require.NoError(t, err)

	got, err := repo.SetProject(ctx, "w1", "p2")
	require.NoError(t, err)
	assert.Equal(t, "p2", got.ProjectID)
	assert.Equal(t, "r1", got.RepoID)
	assert.Equal(t, "/tmp/tree/worktree", got.WorktreePath)

	// ListInRepo reads the store PROJECTION, which trails the Send. Drain it
	// first: the barrier is the write actually landing, not a guess at how long
	// it takes.
	workspace.WaitQuiescentForTest(repo)
	scoped, err := repo.ListInRepo(ctx, "p2", "r1")
	require.NoError(t, err)
	require.Len(t, scoped, 1, "the repo-scoped read finds it under its new project")
}

func TestWorkspace_SetProject_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)

	_, err := repo.SetProject(ctx, "no-such", "p2")
	assert.Error(t, err)
}

// TestRegression_SyncProviderState_UnchangedPollAppendsNothing pins the guard on
// the 5-minute sweep. Without it every visit appended an event and a snapshot
// whether or not the provider had moved — 43,970 of 44,068 provider_synced events
// in a real home carried an empty patch set — and each one bumped the aggregate
// version that OCC and the delete cascade contend on.
//
// The polls below differ only in their timestamp, which is the sweep's steady
// state and the case a version-bumping no-op hides in.
func TestRegression_SyncProviderState_UnchangedPollAppendsNothing(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo := buildRepo(t, ad)
	ctx := context.Background()
	es := ad.WorkspaceES()
	now := time.Unix(1000, 0).UTC()

	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	events := func() int {
		t.Helper()
		// asynx keys the log "events:<id>" and versions from 1 (see its reader).
		evs, readErr := es.ReadFrom(ctx, "events:w1", 1)
		require.NoError(t, readErr)
		return len(evs)
	}

	in := workspace.ProviderInput{
		ID: "w1", HasPR: true, PRStatus: "open", PRUrl: "u", PRTitle: "t",
	}
	_, err = repo.SyncProviderState(ctx, in, now)
	require.NoError(t, err)
	settled := events()

	for i := range 10 {
		got, syncErr := repo.SyncProviderState(ctx, in, now.Add(time.Duration(i)*time.Minute))
		require.NoError(t, syncErr)
		assert.Equal(t, domain.WorkspaceStatusPROpen, got.Status)
	}
	assert.Equal(t, settled, events(), "an unchanged provider poll must append no event")

	// The guard must not swallow a real change.
	in.PRStatus = "merged"
	changed, err := repo.SyncProviderState(ctx, in, now)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusPRMerged, changed.Status)
	assert.Greater(t, events(), settled, "a changed provider poll must still append")
}
