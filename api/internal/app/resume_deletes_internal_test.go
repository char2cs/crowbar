package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fixedPending []project.PendingDelete

func (f fixedPending) PendingDeletes(context.Context) ([]project.PendingDelete, error) { return f, nil }

// Each resumed delete is an operation that has settled when boot goes on
// (invariant D5); one that fails again is a recorded failure, not a log line.
func TestResumeDeletes_ResumedDeletesSettleBeforeItReturns(t *testing.T) {
	ops, _ := recordingOps(t)
	ran := 0
	pending := fixedPending{
		{
			Kind: domain.OpRepoDelete, Ref: domain.EntityRef{Kind: domain.EntityRepo, ID: "r1"},
			ProjectID: "p1", Label: "Remove alpha",
			Run: func(context.Context) error { ran++; return nil },
		},
		{
			Kind: domain.OpProjectDelete, Ref: domain.EntityRef{Kind: domain.EntityProject, ID: "p2"},
			ProjectID: "p2", Label: "Remove beta",
			Run: func(context.Context) error { ran++; return errors.New("worktree wedged") },
		},
	}

	require.NoError(t, resumeDeletes(t.Context(), pending, ops))

	assert.Equal(t, 2, ran)
	rows, err := ops.Snapshot(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byLabel := map[string]domain.Operation{}
	for _, op := range rows {
		byLabel[op.Label] = op
	}
	assert.Equal(t, domain.OpSucceeded, byLabel["Remove alpha"].State)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityRepo, ID: "r1"}, byLabel["Remove alpha"].Subject)
	require.Len(t, byLabel["Remove alpha"].Items, 1)
	assert.Equal(t, domain.OpSucceeded, byLabel["Remove alpha"].Items[0].State, "a finished delete's item finished with it")
	failed := byLabel["Remove beta"]
	require.Len(t, failed.Items, 1)
	assert.Equal(t, domain.OpFailed, failed.Items[0].State)
	assert.Equal(t, failed.Error, failed.Items[0].Error)
	assert.Equal(t, domain.OpFailed, failed.State)
	require.NotNil(t, failed.Error)
	assert.Contains(t, failed.Error.Message, "worktree wedged")
	assert.Equal(t, domain.OpProjectDelete, failed.Kind)
}

// Only a delete that cannot be recorded stops the boot.
func TestResumeDeletes_AnUnrecordableDeleteStopsTheBoot(t *testing.T) {
	ops, _ := recordingOps(t)
	require.NoError(t, ops.Shutdown(t.Context()))
	pending := fixedPending{{
		Kind: domain.OpRepoDelete, Ref: domain.EntityRef{Kind: domain.EntityRepo, ID: "r1"},
		Run: func(context.Context) error { return nil },
	}}

	require.ErrorContains(t, resumeDeletes(t.Context(), pending, ops), "r1")
}
