package activity_test

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func sender(errs ...error) (func(context.Context, asynxModels.Command[domain.ChatActivity]) (asynxModels.Event[domain.ChatActivity], error), *int) {
	calls := 0
	return func(context.Context, asynxModels.Command[domain.ChatActivity]) (asynxModels.Event[domain.ChatActivity], error) {
		var err error
		if calls < len(errs) {
			err = errs[calls]
		}
		calls++
		return asynxModels.Event[domain.ChatActivity]{}, err
	}, &calls
}

func TestDispatch_SucceedsOnTheFirstAttempt(t *testing.T) {
	send, calls := sender()

	err := activity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	require.NoError(t, err)
	assert.Equal(t, 1, *calls)
}

func TestDispatch_NeverRetriesValidation(t *testing.T) {
	send, calls := sender(asynxModels.ErrValidation)

	err := activity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Equal(t, 1, *calls)
}

func TestDispatch_TreatsAFullQueueAsBackpressure(t *testing.T) {
	send, calls := sender(asynxModels.ErrQueueFull)

	err := activity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, apperr.ErrUnavailable)
	assert.Equal(t, 1, *calls)
}

func TestDispatch_RetriesAVersionCollisionUntilItConverges(t *testing.T) {
	send, calls := sender(asynxModels.ErrPipelineFailed, asynxModels.ErrPipelineFailed)

	err := activity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	require.NoError(t, err)
	assert.Equal(t, 3, *calls)
}

func TestDispatch_GivesUpAfterTheRetryBound(t *testing.T) {
	persistent := make([]error, activity.MaxOCCAttempts+5)
	for i := range persistent {
		persistent[i] = asynxModels.ErrPipelineFailed
	}
	send, calls := sender(persistent...)

	err := activity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
	assert.Equal(t, activity.MaxOCCAttempts, *calls)
}

func TestDispatch_SurfacesAnyOtherFailureAsIs(t *testing.T) {
	sentinel := errors.New("disk gone")
	send, calls := sender(sentinel)

	err := activity.DispatchForTest(context.Background(), send, commands.Abandon{ChatID: "c1"})

	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, *calls)
}
