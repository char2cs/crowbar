package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	accmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// frame is a captured hub broadcast: the (chatID, workspaceID, kind)
// lifecycle triple the hub projection hands to a BroadcastFunc.
type frame struct {
	chatID      string
	workspaceID string
	kind        string
	// working is the aggregate's folded busy state as of the event — the field the
	// FE's spinner reads straight off the wire instead of re-deriving from kind.
	working bool
}

// captureHub is a BroadcastFunc double that records every frame it receives,
// safe for concurrent pushes from asynx's per-shard event-processing
// goroutines.
type captureHub struct {
	mu     sync.Mutex
	frames []frame
}

func (h *captureHub) push(chatID, workspaceID, kind string, working bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = append(h.frames, frame{
		chatID:      chatID,
		workspaceID: workspaceID,
		kind:        kind,
		working:     working,
	})
}

func (h *captureHub) all() []frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]frame(nil), h.frames...)
}

func createCmd(chatID string) accmds.Create {
	return accmds.Create{
		ID: chatID, WorkspaceID: "w1", Now: time.Unix(1, 0).UTC(),
	}
}

// TestHubProjection_Create_BroadcastsCreatedFrame proves a single command
// against a fresh chat yields exactly one captured (chatID, "created") frame,
// asserted right after SendWait returns — asynx guarantees every subscriber
// handler (including the hub projection) has completed by then, so no sleep
// or poll is needed.
func TestHubProjection_Create_BroadcastsCreatedFrame(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	_, err := ax.SendWait(ctx, createCmd("c1"))
	require.NoError(t, err)

	frames := h.all()
	require.Len(t, frames, 1)
	assert.Equal(t, frame{chatID: "c1", workspaceID: "w1", kind: "created"}, frames[0])
}

// TestHubProjection_EmitsWorkspaceID proves the hub projection reads the
// aggregate's WorkspaceID off the reduced event (evt.Aggregate.WorkspaceID) and
// passes it through to the broadcast frame — the seam Task 3 will filter the WS
// feed on. Asserted right after SendWait returns, the same deterministic drain
// every other hub-projection test in this file relies on (no sleep/poll).
func TestHubProjection_EmitsWorkspaceID(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	cmd := createCmd("c1")
	cmd.WorkspaceID = "ws-42"
	_, err := ax.SendWait(ctx, cmd)
	require.NoError(t, err)

	frames := h.all()
	require.Len(t, frames, 1)
	assert.Equal(t, "ws-42", frames[0].workspaceID)
}

// TestHubProjection_TurnToggle_BroadcastsStartedThenStopped drives a turn
// start/stop pair and asserts both lifecycle frames land, in order, with the
// expected kinds — the "turn toggle" case called out by the task brief.
func TestHubProjection_TurnToggle_BroadcastsStartedThenStopped(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	_, err := ax.SendWait(ctx, createCmd("c1"))
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, accmds.StartTurn{ChatID: "c1", Now: time.Unix(2, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, accmds.StopTurn{ChatID: "c1", Now: time.Unix(3, 0).UTC()})
	require.NoError(t, err)

	frames := h.all()
	require.Len(t, frames, 3)
	assert.Equal(t, frame{chatID: "c1", workspaceID: "w1", kind: "created"}, frames[0])
	// working rides the frame, folded by the aggregate: the FE spinner reads this and
	// never re-derives it from the kind. A plain StopTurn reports no outstanding async
	// work, so it lands idle.
	assert.Equal(t,
		frame{chatID: "c1", workspaceID: "w1", kind: "turn_started", working: true},
		frames[1])
	assert.Equal(t,
		frame{chatID: "c1", workspaceID: "w1", kind: "turn_stopped", working: false},
		frames[2])
}

// TestHubProjection_TurnStoppedWithAsyncWork_BroadcastsStillWorking is THE bug, at the
// wire: claude hands work to a background subagent and ends its turn right there, so a
// turn_stopped frame must still say working=true or the chat row goes dark under an
// agent that is very much alive.
//
// It is the one frame in the vocabulary whose kind and whose meaning disagree, which is
// exactly why the FE must not infer the spinner from the kind — and why this asserts the
// FIELD, not the kind.
func TestHubProjection_TurnStoppedWithAsyncWork_BroadcastsStillWorking(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	_, err := ax.SendWait(ctx, createCmd("c1"))
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, accmds.StartTurn{ChatID: "c1", Now: time.Unix(2, 0).UTC()})
	require.NoError(t, err)
	// The CLI went quiet with 2 units still running — the level it reported on its Stop.
	_, err = ax.SendWait(ctx, accmds.StopTurn{ChatID: "c1", Now: time.Unix(3, 0).UTC(), AsyncWork: 2})
	require.NoError(t, err)

	frames := h.all()
	require.Len(t, frames, 3)
	assert.Equal(t,
		frame{chatID: "c1", workspaceID: "w1", kind: "turn_stopped", working: true},
		frames[2],
		"a turn that ended with async work outstanding must still broadcast working")
}

// TestHubProjection_AsyncWorkDrains_BroadcastsIdle is the other half, and the one that
// keeps the fix honest: the spinner must actually STOP. The next turn_stop restates the
// level as 0 and the chat goes idle — no counter to unwind, no pairing to get right.
func TestHubProjection_AsyncWorkDrains_BroadcastsIdle(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	_, err := ax.SendWait(ctx, createCmd("c1"))
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, accmds.StartTurn{ChatID: "c1", Now: time.Unix(2, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, accmds.StopTurn{ChatID: "c1", Now: time.Unix(3, 0).UTC(), AsyncWork: 2})
	require.NoError(t, err)
	// claude is re-invoked when the work reports back, and ends THAT turn with nothing
	// left outstanding — the trace's [running,running] → [] transition.
	_, err = ax.SendWait(ctx, accmds.StartTurn{ChatID: "c1", Now: time.Unix(4, 0).UTC()})
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, accmds.StopTurn{ChatID: "c1", Now: time.Unix(5, 0).UTC(), AsyncWork: 0})
	require.NoError(t, err)

	frames := h.all()
	assert.Equal(t,
		frame{chatID: "c1", workspaceID: "w1", kind: "turn_stopped", working: false},
		frames[len(frames)-1],
		"once the reported level drains to zero the spinner must stop")
}

// TestHubProjection_ForgetEmitsScopedDeleted proves the HARD-delete path
// (Task 5, ax.Forget) broadcasts a scoped "deleted" frame carrying the chat's
// workspace id, so every workspace client drops the chat live even though
// Forget hard-erases the aggregate (no read-model row survives to answer a
// later query). Forget itself is the deterministic drain signal: like
// SendWait, it does not return until every subscriber handler on the forget
// event — including this hub projection's OnForget — has run, so asserting on
// h.all() immediately after is safe with no sleep/poll.
func TestHubProjection_ForgetEmitsScopedDeleted(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	cmd := createCmd("c1")
	cmd.WorkspaceID = "ws-9"
	_, err := ax.SendWait(ctx, cmd)
	require.NoError(t, err)

	require.NoError(t, ax.Forget(ctx, "c1"))

	frames := h.all()
	require.Len(t, frames, 2)
	assert.Equal(t, frame{chatID: "c1", workspaceID: "ws-9", kind: "created"}, frames[0])
	assert.Equal(t, frame{chatID: "c1", workspaceID: "ws-9", kind: "deleted"}, frames[1])
}

func TestRegisterHubProjection_SubscribeError(t *testing.T) {
	err := registerHubProjection(&fakeAx{subscribeErr: errors.New("bus down")}, func(string, string, string, bool) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat hub projection: subscribe")
}

// TestRegisterHubProjection_OnForgetError proves an OnForget subscribe
// failure surfaces wrapped, mirroring registerStoreProjection's own
// OnForget-error test.
func TestRegisterHubProjection_OnForgetError(t *testing.T) {
	err := registerHubProjection(&fakeAx{forgetErr: errors.New("bus down")}, func(string, string, string, bool) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat hub projection: onforget")
}

// TestHubProjector_OnEvent_DerivesKindFromEventName unit-tests eventKind's
// extraction directly against the hub projector, independent of asynx wiring,
// and proves the workspace id is read off evt.Aggregate.WorkspaceID (not the
// event name/id) and passed through untouched.
func TestHubProjector_OnEvent_DerivesKindFromEventName(t *testing.T) {
	var got frame
	p := &hubProjector{broadcast: func(chatID, workspaceID, kind string, _ bool) {
		got = frame{chatID: chatID, workspaceID: workspaceID, kind: kind}
	}}
	p.onEvent(context.Background(), asynxModels.Event[domain.AgentChat]{
		AggregateID: "c9",
		EventName:   "agentchat.segment_opened.c9",
		Aggregate:   domain.AgentChat{WorkspaceID: "w9"},
	})
	assert.Equal(t, frame{chatID: "c9", workspaceID: "w9", kind: "segment_opened"}, got)
}

// TestEventKind_UnrecognizedShape falls back to the name (minus the
// aggregate prefix) rather than dropping the frame, for any future event
// name that doesn't fit the "agentchat.<kind>.<id>" pattern.
func TestEventKind_UnrecognizedShape(t *testing.T) {
	assert.Equal(t, "weird", eventKind("agentchat.weird"))
}
