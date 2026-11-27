package workspace_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestConcurrentSends_ToOneWorkspaceAllCommitInTurn(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1", Provisioning: domain.WorkspacePlaceholder}, now)
	require.NoError(t, err)

	const n = 20
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = repo.SyncWorkingTreeState(ctx, workspace.SyncInput{ID: "w1", Added: idx}, now)
		}(i)
	}
	wg.Wait()

	failed := 0
	for _, e := range errs {
		if e != nil {
			failed++
		}
	}
	require.Zero(t, failed, "%d/%d concurrent same-aggregate commands failed", failed, n)

	got, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.ID)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status)
}

func TestConcurrentHomeCreates_OneProjectNeverGetsTwoHomeWorkspaces(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(2000, 0).UTC()

	const n = 20
	id := workspace.ProjectHomeID("proj-home-race")
	var wins atomic.Int32
	ready := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, err := repo.Create(ctx, workspace.CreateInput{
				ID: id, ProjectID: "proj-home-race", WorktreePath: "/projects/home-race",
				Kind:         domain.WorkspaceKindHome,
				Provisioning: domain.WorkspaceShared,
			}, now)
			if err == nil {
				wins.Add(1)
			}
		}()
	}
	close(ready)
	wg.Wait()
	assert.Equal(t, int32(1), wins.Load(), "exactly one create wins; the aggregate refuses the rest")
	first := id

	rows := listQuiescent(t, ctx, repo, 1)
	assert.Equal(t, first, rows[0].ID)
	assert.Equal(t, domain.WorkspaceKindHome, rows[0].Kind)
}
