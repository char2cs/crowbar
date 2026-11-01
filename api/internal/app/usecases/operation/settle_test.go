package operation_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func itemSpec(id string) operation.Spec {
	spec := pushSpec(id)
	spec.Items = []domain.OperationItem{{Label: "a", State: domain.OpRunning}, {Label: "b", State: domain.OpRunning}}
	return spec
}

func TestService_WorkThatReturnsFinishesItsOpenItems(t *testing.T) {
	f := newFixture(t)

	op, err := f.svc.Run(context.Background(), itemSpec(""), succeed)
	require.NoError(t, err)

	for _, item := range op.Items {
		assert.Equal(t, domain.OpSucceeded, item.State, item.Label)
	}
	assert.Equal(t, op.Items, f.row(t, op.ID).Items)
}

func TestService_AFailedItemLeavesTheOthersFinishedWhenTheWorkReturns(t *testing.T) {
	f := newFixture(t)
	taken := &problem.Problem{Code: problem.Conflict, Message: "taken"}

	op, err := f.svc.Run(context.Background(), itemSpec(""), func(_ context.Context, p operation.Progress) error {
		p.Item(domain.OperationItem{Label: "a", State: domain.OpRunning, Error: taken})
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, problem.Partial, op.Error.Code)
	assert.Equal(t, domain.OpFailed, op.Items[0].State)
	assert.Equal(t, taken, op.Items[0].Error)
	assert.Equal(t, domain.OpSucceeded, op.Items[1].State)
}

func TestService_FailedWorkFailsItsOpenItemsWithItsReason(t *testing.T) {
	f := newFixture(t)

	op, err := f.svc.Run(context.Background(), itemSpec(""), func(_ context.Context, p operation.Progress) error {
		p.Item(domain.OperationItem{Label: "a", State: domain.OpSucceeded})
		return problem.New(problem.Conflict, "held")
	})
	require.NoError(t, err)

	assert.Equal(t, domain.OpSucceeded, op.Items[0].State, "a finished item keeps its outcome")
	assert.Equal(t, domain.OpFailed, op.Items[1].State)
	assert.Equal(t, op.Error, op.Items[1].Error)
}

func TestService_InterruptedWorkFailsItsOpenItemsAsInterrupted(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	started := make(chan struct{})
	_, err := f.svc.Start(context.Background(), itemSpec(id), func(ctx context.Context, _ operation.Progress) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	require.NoError(t, err)
	<-started

	require.NoError(t, f.svc.Shutdown(context.Background()))

	for _, item := range f.row(t, id).Items {
		assert.Equal(t, domain.OpFailed, item.State, item.Label)
		require.NotNil(t, item.Error, item.Label)
		assert.Equal(t, problem.Interrupted, item.Error.Code, item.Label)
	}
}

func TestService_FailRecordsItsItemsFailed(t *testing.T) {
	f := newFixture(t)

	op, err := f.svc.Fail(context.Background(), itemSpec(""), problem.New(problem.Unavailable, "disk gone"))
	require.NoError(t, err)

	for _, item := range op.Items {
		assert.Equal(t, domain.OpFailed, item.State, item.Label)
		assert.Equal(t, op.Error, item.Error, item.Label)
	}
}
