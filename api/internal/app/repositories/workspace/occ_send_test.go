package workspace_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestWorkspace_OccSendErrorDisposition pins the terminal error-disposition
// contract (spec §3.5, decision 10) against a fake send, mirroring the
// reviewthread package's own OccSend contract test: ErrPipelineFailed is
// retried up to MaxOCCAttempts then surfaced; ErrValidation and ErrQueueFull
// are never retried; an unclassified error is neither retried nor translated.
func TestWorkspace_OccSendErrorDisposition(t *testing.T) {
	ctx := context.Background()
	cmd := commands.TouchActivity{ID: "w1", Now: time.Unix(1, 0)}

	t.Run("ErrPipelineFailed retried then surfaced", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			calls++
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
		}
		_, err := workspace.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
		assert.Equal(t, workspace.MaxOCCAttempts, calls)
	})

	t.Run("ErrValidation never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			calls++
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("nope: %w", asynxModels.ErrValidation)
		}
		_, err := workspace.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrValidation)
		assert.Equal(t, 1, calls)
	})

	t.Run("ErrQueueFull translated to unavailable, never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			calls++
			return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("full: %w", asynxModels.ErrQueueFull)
		}
		_, err := workspace.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, apperr.ErrUnavailable)
		assert.Equal(t, 1, calls)
	})

	t.Run("unclassified error surfaced immediately, never retried", func(t *testing.T) {
		sentinel := fmt.Errorf("unclassified failure")
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
			calls++
			return asynxModels.Event[domain.Workspace]{}, sentinel
		}
		_, err := workspace.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, 1, calls, "an unclassified error must not be retried")
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

// TestWorkspace_OccSend_AbortsTheBackoffWaitOnContextCancellation proves the OCC
// retry loop's backoff is itself cancellable: a caller that cancels its context
// mid-retry must see ctx.Err() rather than block for the full jittered wait, or
// worse, retry indefinitely against a caller that has already given up.
func TestWorkspace_OccSend_AbortsTheBackoffWaitOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := commands.TouchActivity{ID: "w1", Now: time.Unix(1, 0)}

	calls := 0
	send := func(context.Context, asynxModels.Command[domain.Workspace]) (asynxModels.Event[domain.Workspace], error) {
		calls++
		if calls == 1 {
			// Cancel BEFORE the loop's first backoff wait so occBackoff's
			// ctx.Done() case is guaranteed to win the select over its timer,
			// however small the jittered window happened to draw.
			cancel()
		}
		return asynxModels.Event[domain.Workspace]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
	}

	_, err := workspace.OccSend(ctx, send, cmd)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls, "a cancelled context must abort during the backoff wait, not after another send attempt")
}
