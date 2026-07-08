package workspace_test

import (
	"context"
	"fmt"
	"sync"
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
