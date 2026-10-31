//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func isReady(f stream.Frame) bool { return f.T == stream.FrameReady }

func isSynced(scope string) func(stream.Frame) bool {
	return func(f stream.Frame) bool { return f.T == stream.FrameSynced && f.Scope == scope }
}

func TestStream_FirstFrameIsReady(t *testing.T) {
	h := newHarness(t)
	c := dialStream(t, h)

	first := c.Next(t, func(stream.Frame) bool { return true })

	assert.Equal(t, stream.FrameReady, first.T)
	assert.NotEmpty(t, first.Epoch)
}

// A new daemon's versions start over, so the client must be told to drop what it holds.
func TestStream_ReadyCarriesANewEpochAfterRestart(t *testing.T) {
	h := newHarness(t)
	before := dialStream(t, h).Next(t, isReady).Epoch
	h.shutdown()

	again := newHarnessAt(t, h.home)
	after := dialStream(t, again).Next(t, isReady).Epoch

	assert.NotEmpty(t, after)
	assert.NotEqual(t, before, after)
}

func TestStream_ProjectsScopeSnapshotsOperations(t *testing.T) {
	h := newHarness(t)
	push := failingPush(t, h)
	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	awaitOperation(t, h, op.ID, operationSettled)

	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	var snapshot *domain.Operation
	c.Next(t, func(f stream.Frame) bool {
		if isOperationFrame(f, op.ID) {
			got := operationIn(t, f)
			snapshot = &got
		}
		return isSynced("projects")(f)
	})

	require.NotNil(t, snapshot, "the settled operation arrives before synced")
	assert.Equal(t, domain.OpFailed, snapshot.State)
}

func TestStream_AnOperationFrameFollowsItsStateChanges(t *testing.T) {
	h := newHarness(t)
	push := failingPush(t, h)
	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	c.Next(t, isSynced("projects"))

	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	var states []domain.OperationState
	var last uint64
	c.Next(t, func(f stream.Frame) bool {
		if !isOperationFrame(f, op.ID) {
			return false
		}
		assert.Greater(t, f.V, last, "each frame of the operation carries a newer version")
		last = f.V
		got := operationIn(t, f)
		states = append(states, got.State)
		return operationSettled(got)
	})

	assert.Equal(t, []domain.OperationState{domain.OpAccepted, domain.OpRunning, domain.OpFailed}, states)
}

func TestStream_AnUnknownScopeIsRefusedWithAProblem(t *testing.T) {
	h := newHarness(t)
	c := dialStream(t, h)
	c.Sub(t, "nope:x", 0)

	closed := c.Next(t, func(f stream.Frame) bool { return f.T == stream.FrameClosed })

	assert.Equal(t, "nope:x", closed.Scope)
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	require.NotNil(t, closed.Problem)
	assert.Equal(t, problem.InvalidArgument, closed.Problem.Code)

	c.Sub(t, "projects", 0)
	c.Next(t, isSynced("projects")) // the connection still serves
}
