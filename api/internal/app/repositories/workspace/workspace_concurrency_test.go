package workspace_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestConcurrentSends_NoWriteMu_OCC proves the writeMu deletion (decision 10):
// many commands targeting the SAME workspace aggregate at once must ALL commit
// without a hand-rolled single-writer mutex. Per-aggregate safety is now shard
// routing + (id,version) uniqueness + OCC retry — the version losers retry (Send
// re-reads the current version each attempt) and converge, so none escapes with
// ErrPipelineFailed.
func TestConcurrentSends_NoWriteMu_OCC(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
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
	require.Zero(t, failed, "%d/%d concurrent same-aggregate commands failed under OCC (no writeMu)", failed, n)

	// The aggregate is consistent after the storm (folds cleanly from the log).
	got, err := repo.Get(ctx, "w1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.ID)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status)
}

// TestConcurrentHomeCreates_OneProjectNeverGetsTwoHomeWorkspaces is the
// regression for a live bug: two requests racing "create the home for
// project P," with nothing serializing them, used to both read no home
// workspace yet and both mint a fresh random id — a random id gives asynx's
// own per-aggregate concurrency control nothing to enforce, since the two
// calls were never contending for the same aggregate, so BOTH succeeded and
// the project ended up with two home workspaces. Downstream, nothing ever
// reconciled that: each caller's own response carried whichever one IT just
// made, and the frontend that cached the loser's id could never create a
// thread again — every attempt failing "asynx: aggregate not found."
//
// A project's home now has a DETERMINISTIC id (ProjectHomeID's own doc), so
// every one of these n concurrent creates targets the SAME aggregate — this proves that, against the REAL asynx
// event store (no mocks, no injected sleep needed to widen a window: a
// deterministic id means there IS no window, only a serialized queue), not
// a hand-rolled simulation of it.
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
				Kind: domain.WorkspaceKindHome,
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

	// And the READ MODEL agrees there is exactly one, not two rows that
	// happen to share an id in the return values but not in storage.
	rows := listQuiescent(t, ctx, repo, 1)
	assert.Equal(t, first, rows[0].ID)
	assert.Equal(t, domain.WorkspaceKindHome, rows[0].Kind)
}

// TestSendWithOCC_ErrorDisposition pins the terminal error-disposition contract
// (spec §3.5, decision 10) against a fake send: ErrPipelineFailed is retried
// exactly MaxOCCAttempts then surfaced (→ 409); ErrValidation is never retried
// (→ 422); ErrQueueFull is never retried and is translated to
// apperr.ErrUnavailable (→ 503). All via errors.Is.
func TestSendWithOCC_ErrorDisposition(t *testing.T) {
	ctx := context.Background()
	cmd := wscmds.TouchActivity{ID: "w1"}

	t.Run("ErrPipelineFailed retried then surfaced", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			calls++
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
		}
		_, err := workspace.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
		assert.Equal(t, workspace.MaxOCCAttempts, calls, "ErrPipelineFailed retried exactly MaxOCCAttempts")
	})

	t.Run("ErrValidation never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			calls++
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("nope: %w", asynxModels.ErrValidation)
		}
		_, err := workspace.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrValidation)
		assert.Equal(t, 1, calls, "ErrValidation must not be retried")
	})

	t.Run("ErrQueueFull translated to unavailable, never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			calls++
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("full: %w", asynxModels.ErrQueueFull)
		}
		_, err := workspace.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, apperr.ErrUnavailable)
		assert.Equal(t, 1, calls, "ErrQueueFull must not be retried")
	})

	t.Run("success returns immediately", func(t *testing.T) {
		send := func(_ context.Context, c asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			return asynxModels.Event[domain.Workspace]{Aggregate: domain.Workspace{ID: c.AggregateID()}}, nil
		}
		evt, err := workspace.OccSend(ctx, send, cmd)
		require.NoError(t, err)
		assert.Equal(t, "w1", evt.Aggregate.ID)
	})
}
