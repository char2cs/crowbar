package operation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func seed(t *testing.T, f fixture, id string, state domain.OperationState, updated time.Time) {
	t.Helper()
	require.NoError(t, f.store.Save(context.Background(), domain.Operation{
		ID: id, Kind: domain.OpGitPush, State: state, Version: 1, CreatedAt: updated, UpdatedAt: updated,
		Subject: domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"},
	}))
}

func TestService_RecoverFailsWhatARestartInterrupted(t *testing.T) {
	f := newFixture(t)
	seed(t, f, "accepted", domain.OpAccepted, epoch.Add(-time.Hour))
	seed(t, f, "running", domain.OpRunning, epoch.Add(-time.Hour))
	seed(t, f, "done", domain.OpSucceeded, epoch.Add(-time.Hour))

	require.NoError(t, f.svc.Recover(context.Background()))

	want := problem.Problem{Code: problem.Interrupted, Message: "Crowbar stopped before this finished"}
	for _, id := range []string{"accepted", "running"} {
		row := f.row(t, id)
		assert.Equal(t, domain.OpFailed, row.State, id)
		require.NotNil(t, row.Error, id)
		assert.Equal(t, want, *row.Error, id)
		assert.Equal(t, 2, row.Version, id)
	}
	assert.Equal(t, domain.OpSucceeded, f.row(t, "done").State)
	assert.Nil(t, f.row(t, "done").Error)
}

func TestService_RecoverKeepsTheNewest500Settled(t *testing.T) {
	f := newFixture(t)
	for i := range 503 {
		seed(t, f, fmt.Sprintf("op-%03d", i), domain.OpFailed, epoch.Add(-time.Duration(1000-i)*time.Minute))
	}
	seed(t, f, "old-running", domain.OpRunning, epoch.Add(-48*time.Hour))

	require.NoError(t, f.svc.Recover(context.Background()))

	all, err := f.store.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 500)
	// 504 settled once the running row is failed: the four oldest go.
	for _, gone := range []string{"op-000", "op-001", "op-002", "op-003"} {
		got, err := f.store.FindByKey(context.Background(), gone)
		require.NoError(t, err)
		assert.Nil(t, got, gone)
	}
	assert.Equal(t, "op-004", f.row(t, "op-004").ID)
	assert.Equal(t, domain.OpFailed, f.row(t, "old-running").State, "an interrupted row is never pruned at the boot that settles it")
}

// An item a restart cut short fails with its operation; one that had finished keeps its outcome.
func TestService_RecoverFailsTheItemsTheRestartCutShort(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.store.Save(context.Background(), domain.Operation{
		ID: "delete", Kind: domain.OpRepoDelete, State: domain.OpRunning, Version: 1, UpdatedAt: epoch,
		Subject: domain.EntityRef{Kind: domain.EntityRepo, ID: "r1"},
		Items: []domain.OperationItem{
			{Label: "Remove alpha", State: domain.OpSucceeded},
			{Label: "Remove gamma", State: domain.OpRunning},
		},
	}))

	require.NoError(t, f.svc.Recover(context.Background()))

	items := f.row(t, "delete").Items
	assert.Equal(t, domain.OpSucceeded, items[0].State)
	assert.Nil(t, items[0].Error)
	assert.Equal(t, domain.OpFailed, items[1].State)
	require.NotNil(t, items[1].Error)
	assert.Equal(t, problem.Interrupted, items[1].Error.Code)
}

// A parked save is about the boot that parked it: the restart redelivers the
// aggregate, so the failure is gone at the next boot instead of shown forever.
func TestService_RecoverDropsTheSavesTheLastBootParked(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.store.Save(context.Background(), domain.Operation{
		ID: "parked", Kind: domain.OpEntitySave, State: domain.OpFailed, Version: 1,
		CreatedAt: epoch.Add(-time.Hour), UpdatedAt: epoch.Add(-time.Hour),
		Subject: domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"},
		Error:   &problem.Problem{Code: problem.Unavailable, Message: "Crowbar stopped saving this workspace's changes until it restarts"},
	}))
	seed(t, f, "push", domain.OpFailed, epoch)
	thisBoot, err := f.svc.Fail(context.Background(), operation.Spec{
		Kind: domain.OpEntitySave, Subject: domain.EntityRef{Kind: domain.EntityChat, ID: "c1"}, Label: "Save chat",
	}, problem.New(problem.Unavailable, "parked again"))
	require.NoError(t, err)

	require.NoError(t, f.svc.Recover(context.Background()))

	got, err := f.store.FindByKey(context.Background(), "parked")
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Equal(t, domain.OpFailed, f.row(t, thisBoot.ID).State, "a save this boot parked is still news")
	assert.Equal(t, domain.OpFailed, f.row(t, "push").State, "any other failure stays until the user dismisses it")
}
