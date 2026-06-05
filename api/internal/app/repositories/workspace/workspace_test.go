package workspace_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func newRepo(
	t *testing.T,
) (context.Context, workspace.Workspace) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), workspace.New(ax)
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

func TestWorkspace_SyncClearsNewStatus(t *testing.T) {
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
	assert.Equal(t, domain.WorkspaceStatus(""), synced.Status)
	assert.Equal(t, 10, synced.Added)

	reloaded, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatus(""), reloaded.Status)
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
	assert.Equal(t, domain.WorkspaceStatusPROpen, got.Status)
	assert.True(t, got.Locked)
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

func TestWorkspace_Reparent_TouchActivity_ForkPoint_Pending(t *testing.T) {
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
	pm, err := repo.SetPendingMerge(ctx, "w1", gitdomain.MergeStrategyMerge, "p2")
	require.NoError(t, err)
	require.NotNil(t, pm.PendingMerge)
	cl, err := repo.ClearPendingMerge(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, cl.PendingMerge)
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

func TestWorkspace_SetPendingMerge_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.SetPendingMerge(ctx, "no-such", gitdomain.MergeStrategyMerge, "p")
	assert.Error(t, err)
}

func TestWorkspace_ClearPendingMerge_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.ClearPendingMerge(ctx, "no-such")
	assert.Error(t, err)
}

func TestWorkspace_Delete_ErrorOnMissing(t *testing.T) {
	ctx, repo := newRepo(t)
	err := repo.Delete(ctx, "no-such")
	assert.Error(t, err)
}
