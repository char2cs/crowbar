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
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
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
) (workspace.Workspace, wspaths.WorkspacePaths) {
	t.Helper()
	pathsStore, err := wspaths.NewWorkspacePaths(ad.GlobalView())
	require.NoError(t, err)
	repo, err := workspace.New(wsAx(t, ad), ad.WorkspaceES(), ad.WorkspaceView(), pathsStore)
	require.NoError(t, err)
	return repo, pathsStore
}

func newRepo(
	t *testing.T,
) (context.Context, workspace.Workspace) {
	t.Helper()
	repo, _ := buildRepo(t, newAdapter(t, t.TempDir()))
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

// TestCreate_WritesPathRow proves §3.9 write-point (a): Create records the
// workspace id→worktree-path row in view.db's rename-resilience map.
func TestCreate_WritesPathRow(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo, pathsStore := buildRepo(t, ad)
	ctx := context.Background()

	_, err := repo.Create(ctx, workspace.CreateInput{
		ID:           "w1",
		RepoID:       "r1",
		ProjectID:    "p1",
		Branch:       "b",
		WorktreePath: "/h/projects/p1/github.com/o/r/b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	got, err := pathsStore.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "/h/projects/p1/github.com/o/r/b", got)
}

// TestRelocate_UpdatesPathRow proves §3.9 write-point (b): relocating a
// workspace moves it on disk, so the id→path index has to follow it. A stale row
// is not cosmetic drift — the delete reactor resolves the directory it rm -rf's
// from the record and this index, so a row still naming the old directory makes
// a later delete destroy whatever now occupies it.
//
// The invariant used to ride on RenameBranch, back when a branch name change
// moved the tree. It does not any more (the root is keyed by workspace id), so
// it belongs to the one command that still changes a path.
func TestRelocate_UpdatesPathRow(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo, pathsStore := buildRepo(t, ad)
	ctx := context.Background()

	_, err := repo.Create(ctx, workspace.CreateInput{
		ID:           "w1",
		RepoID:       "r1",
		ProjectID:    "p1",
		Branch:       "a",
		WorktreePath: "/h/projects/p1/github.com/o/r/a/worktree",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	_, err = repo.Relocate(ctx, "w1", "/h/projects/p1/github.com/o/r/b/worktree")
	require.NoError(t, err)

	got, err := pathsStore.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "/h/projects/p1/github.com/o/r/b/worktree", got)
}

// A rename the aggregate refuses must leave the index exactly as it was: the
// record still names the old path, and the index has to agree with it rather
// than advertise a move that never happened.
func TestRelocate_LeavesPathRowIntactWhenRecordWriteFails(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo, pathsStore := buildRepo(t, ad)
	ctx := context.Background()

	_, err := repo.Create(ctx, workspace.CreateInput{
		ID:           "w1",
		RepoID:       "r1",
		ProjectID:    "p1",
		Branch:       "a",
		WorktreePath: "/h/projects/p1/github.com/o/r/a/worktree",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	// A relocate with no destination is refused by the command.
	_, err = repo.Relocate(ctx, "w1", "")
	require.Error(t, err)

	got, err := pathsStore.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "/h/projects/p1/github.com/o/r/a/worktree", got)
}

// A workspace whose index row was never written (or was already purged) has no
// previous path to restore, so a refused relocate must leave NO row rather than
// invent one pointing at a directory the record does not claim.
func TestRelocate_LeavesNoPathRowWhenThereWasNoneAndTheRecordWriteFails(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo, pathsStore := buildRepo(t, ad)
	ctx := context.Background()

	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "a",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.NoError(t, pathsStore.Delete(ctx, "w1"))

	_, err = repo.Relocate(ctx, "w1", "")
	require.Error(t, err)

	_, getErr := pathsStore.Get(ctx, "w1")
	assert.ErrorIs(t, getErr, wspaths.ErrNotFound)
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
	paths1, err := wspaths.NewWorkspacePaths(first.GlobalView())
	require.NoError(t, err)
	repo1, err := workspace.New(ax1, first.WorkspaceES(), first.WorkspaceView(), paths1)
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
	repo2, _ := buildRepo(t, second)

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
	pathsStore, err := wspaths.NewWorkspacePaths(ad.GlobalView())
	require.NoError(t, err)

	_, err = workspace.New(nil, ad.WorkspaceES(), ad.WorkspaceView(), pathsStore)
	assert.Error(t, err, "nil asynx must error")

	_, err = workspace.New(wsAx(t, ad), nil, ad.WorkspaceView(), pathsStore)
	assert.Error(t, err, "nil event store must error")

	_, err = workspace.New(wsAx(t, ad), ad.WorkspaceES(), nil, pathsStore)
	assert.Error(t, err, "nil store db must error")

	_, err = workspace.New(wsAx(t, ad), ad.WorkspaceES(), ad.WorkspaceView(), nil)
	assert.Error(t, err, "nil paths store must error")
}

// TestWorkspace_Create_RollsBackPathRowOnFailure proves the Create rollback:
// when the command is rejected (here an empty ProjectID fails CreateWorkspace's
// Validate), the id→path row written before the send is rolled back rather than
// orphaned in the rename-resilience map.
func TestWorkspace_Create_RollsBackPathRowOnFailure(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo, pathsStore := buildRepo(t, ad)
	ctx := context.Background()

	_, err := repo.Create(ctx, workspace.CreateInput{
		ID:           "w1",
		RepoID:       "r1",
		ProjectID:    "", // invalid → CreateWorkspace.Validate rejects
		WorktreePath: "/some/path",
	}, time.Unix(1, 0).UTC())
	require.Error(t, err)

	_, getErr := pathsStore.Get(ctx, "w1")
	assert.ErrorIs(t, getErr, wspaths.ErrNotFound,
		"a failed Create must roll back its id→path row, not orphan it")
}

func TestListInRepo_ScopesToRepo(t *testing.T) {
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
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
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
	pathsStore, err := wspaths.NewWorkspacePaths(ad.GlobalView())
	require.NoError(t, err)
	repo, err := workspace.New(wsAx(t, ad), ad.WorkspaceES(), ad.WorkspaceView(), pathsStore, workspace.WithReconciler(r))
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

// The sidebar's placement write is deliberately narrow: it moves the folder edge
// and the index and leaves the fork lineage exactly as git left it, because
// three read paths resolve ParentID back to a workspace.
func TestWorkspace_SetPlacement_LeavesTheForkLineageAlone(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "feat",
		ParentID: "parent", ForkPointSha: "abc123",
	}, now)
	require.NoError(t, err)

	got, err := repo.SetPlacement(ctx, "w1", "f1", 3)
	require.NoError(t, err)
	assert.Equal(t, "f1", got.FolderID)
	assert.Equal(t, 3, got.Order)
	assert.Equal(t, "parent", got.ParentID)
	assert.Equal(t, "abc123", got.ForkPointSha)

	reloaded, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "f1", reloaded.FolderID)
	assert.Equal(t, 3, reloaded.Order)
}

func TestWorkspace_SetPlacement_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)

	_, err := repo.SetPlacement(ctx, "no-such", "f1", 0)
	assert.Error(t, err)
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
