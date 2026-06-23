package workspace_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

// TestWorkspace_ConcurrentSameAggregateCommandsAllSucceed proves H2: many
// commands targeting the SAME workspace aggregate at once must ALL commit. In
// production the fs-watcher (SyncWorkingTreeState), provider sweep
// (SyncProviderState), chat TouchActivity, and user git ops all fire on one
// workspace within the same tick; without single-writer-per-aggregate
// serialization the optimistic event store rejects the losers with a
// version/PK conflict and the read model silently goes stale.
func TestWorkspace_ConcurrentSameAggregateCommandsAllSucceed(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	const n = 32
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
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
	require.Zero(t, failed, "%d/%d concurrent same-aggregate commands failed with a version conflict", failed, n)
}
