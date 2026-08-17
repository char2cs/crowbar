package agentactivity_test

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func sender(errs ...error) (func(context.Context, asynxModels.Command[domain.AgentActivity]) (asynxModels.Event[domain.AgentActivity], error), *int) {
	calls := 0
	return func(context.Context, asynxModels.Command[domain.AgentActivity]) (asynxModels.Event[domain.AgentActivity], error) {
		var err error
		if calls < len(errs) {
			err = errs[calls]
		}
		calls++
		return asynxModels.Event[domain.AgentActivity]{}, err
	}, &calls
}

func TestDispatch_SucceedsOnTheFirstAttempt(t *testing.T) {
	send, calls := sender()

	err := agentactivity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	require.NoError(t, err)
	assert.Equal(t, 1, *calls)
}

// A validation failure means the command is WRONG, not late. Retrying it would
// burn the whole budget on something that can never succeed.
func TestDispatch_NeverRetriesValidation(t *testing.T) {
	send, calls := sender(asynxModels.ErrValidation)

	err := agentactivity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Equal(t, 1, *calls)
}

// A full shard queue is BACKPRESSURE, not a version race: retrying makes it
// worse. It is surfaced as unavailable so the caller can back off.
func TestDispatch_TreatsAFullQueueAsBackpressure(t *testing.T) {
	send, calls := sender(asynxModels.ErrQueueFull)

	err := agentactivity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
	assert.Equal(t, 1, *calls)
}

// Concurrent hooks for one chat version-collide, and the loser converges because
// each attempt re-reads the current version.
func TestDispatch_RetriesAVersionCollisionUntilItConverges(t *testing.T) {
	send, calls := sender(asynxModels.ErrPipelineFailed, asynxModels.ErrPipelineFailed)

	err := agentactivity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	require.NoError(t, err)
	assert.Equal(t, 3, *calls)
}

func TestDispatch_GivesUpAfterTheRetryBound(t *testing.T) {
	persistent := make([]error, agentactivity.MaxOCCAttempts+5)
	for i := range persistent {
		persistent[i] = asynxModels.ErrPipelineFailed
	}
	send, calls := sender(persistent...)

	err := agentactivity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
	assert.Equal(t, agentactivity.MaxOCCAttempts, *calls)
}

func TestDispatch_SurfacesAnyOtherFailureAsIs(t *testing.T) {
	sentinel := errors.New("disk gone")
	send, calls := sender(sentinel)

	err := agentactivity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, *calls)
}
