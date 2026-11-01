//go:build integration

package tests

import (
	"fmt"
	"net/http"
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

// Review Focus 2: a resume sends every row after since once, in order, then synced at the head.
func TestStream_ChatScopeResumesFromSinceWithoutGapOrDuplicate(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	scope := "chat:" + chatID
	c := dialStream(t, h)
	c.Next(t, isReady)
	c.Sub(t, scope, 0)
	head := c.Next(t, isSynced(scope)).Seq
	require.GreaterOrEqual(t, head, int64(2))
	// The resume is a reconnect: frames the first connection still had in
	// flight cannot reach the second.
	require.NoError(t, c.conn.Close())

	for i := range 3 {
		post("message_delta", delta(fmt.Sprintf("msg-%d", i), 0, true, "HI"))
	}
	c = dialStream(t, h)
	c.Next(t, isReady)
	c.Sub(t, scope, head-1)
	var seqs []int64
	synced := c.Next(t, func(f stream.Frame) bool {
		if f.Scope == scope && f.T == stream.FrameRow {
			seqs = append(seqs, f.Row.Seq)
		}
		return isSynced(scope)(f)
	})

	assert.Equal(t, contiguous(head, head+3), seqs)
	assert.Equal(t, head+3, synced.Seq)
	post("message_delta", delta("msg-live", 0, true, "HI"))
	live := c.Next(t, func(f stream.Frame) bool { return f.Scope == scope && f.T == stream.FrameRow })
	assert.Equal(t, head+4, live.Row.Seq, "live rows follow the replay with no gap")
}

// A pane that lets go of a chat and takes it back on one connection may still
// get a row the first sub had in flight. A client that applies only the row
// after its last one, as the web does, ends with every row once and no gap.
func TestStream_ChatScopeResubscribedOnOneConnectionHasNoGap(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	scope := "chat:" + chatID
	c := dialStream(t, h)
	c.Next(t, isReady)
	c.Sub(t, scope, 0)
	head := c.Next(t, isSynced(scope)).Seq

	c.Unsub(t, scope)
	for i := range 3 {
		post("message_delta", delta(fmt.Sprintf("msg-%d", i), 0, true, "HI"))
	}
	c.Sub(t, scope, head)
	applied, rows := head, []int64{}
	synced := c.Next(t, func(f stream.Frame) bool {
		if f.Scope == scope && f.T == stream.FrameRow && f.Row.Seq == applied+1 {
			applied++
			rows = append(rows, applied)
		}
		return isSynced(scope)(f)
	})

	assert.Equal(t, contiguous(head+1, head+3), rows)
	assert.Equal(t, applied, synced.Seq, "the synced names the last row applied")
}

func TestStream_DeletedChatClosesItsScopeOnly(t *testing.T) {
	h := newHarness(t)
	imported, chatID, _ := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	push := failingPush(t, h)
	scope := "chat:" + chatID
	c := dialStream(t, h)
	c.Hold(t, "projects")
	c.Hold(t, scope)

	_ = h.raw(http.MethodDelete, repoBase(imported)+"/chats/"+chatID, nil, http.StatusOK).Body.Close()
	closed := c.Next(t, func(f stream.Frame) bool { return f.Scope == scope && f.T == stream.FrameClosed })
	assert.Equal(t, stream.Closed(scope, stream.ReasonDeleted, nil), closed)

	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	c.Next(t, func(f stream.Frame) bool { return isOperationFrame(f, op.ID) })
}
