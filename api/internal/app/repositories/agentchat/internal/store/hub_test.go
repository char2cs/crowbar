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

// frame is a captured hub broadcast: the (chatID, kind) lifecycle pair the
// hub projection hands to a BroadcastFunc.
type frame struct {
	chatID string
	kind   string
}

// captureHub is a BroadcastFunc double that records every frame it receives,
// safe for concurrent pushes from asynx's per-shard event-processing
// goroutines.
type captureHub struct {
	mu     sync.Mutex
	frames []frame
}

func (h *captureHub) push(chatID, kind string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = append(h.frames, frame{chatID: chatID, kind: kind})
}

func (h *captureHub) all() []frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]frame(nil), h.frames...)
}

func createCmd(chatID string) accmds.Create {
	return accmds.Create{
		ID: chatID, WorkspaceID: "w1", SegmentID: "s1", CrowbarSegmentID: "cs1",
		ProviderID: "claude", TerminalSession: "term-1", Now: time.Unix(1, 0).UTC(),
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
	assert.Equal(t, frame{chatID: "c1", kind: "created"}, frames[0])
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
	assert.Equal(t, frame{chatID: "c1", kind: "created"}, frames[0])
	assert.Equal(t, frame{chatID: "c1", kind: "turn_started"}, frames[1])
	assert.Equal(t, frame{chatID: "c1", kind: "turn_stopped"}, frames[2])
}

// TestHubProjection_Delete_BroadcastsDeletedFrame proves a tombstoning
// Delete still broadcasts a "deleted" frame, even though the store
// projection's read model drops the chat from list views for the same event.
func TestHubProjection_Delete_BroadcastsDeletedFrame(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	_, err := ax.SendWait(ctx, createCmd("c1"))
	require.NoError(t, err)
	_, err = ax.SendWait(ctx, accmds.Delete{ChatID: "c1"})
	require.NoError(t, err)

	frames := h.all()
	require.Len(t, frames, 2)
	assert.Equal(t, frame{chatID: "c1", kind: "deleted"}, frames[1])
}

func TestRegisterHubProjection_SubscribeError(t *testing.T) {
	err := registerHubProjection(&fakeAx{subscribeErr: errors.New("bus down")}, func(string, string) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat hub projection: subscribe")
}

// TestHubProjector_OnEvent_DerivesKindFromEventName unit-tests eventKind's
// extraction directly against the hub projector, independent of asynx wiring.
func TestHubProjector_OnEvent_DerivesKindFromEventName(t *testing.T) {
	var got frame
	p := &hubProjector{broadcast: func(chatID, kind string) { got = frame{chatID: chatID, kind: kind} }}
	p.onEvent(context.Background(), asynxModels.Event[domain.AgentChat]{
		AggregateID: "c9",
		EventName:   "agentchat.segment_opened.c9",
	})
	assert.Equal(t, frame{chatID: "c9", kind: "segment_opened"}, got)
}

// TestEventKind_UnrecognizedShape falls back to the name (minus the
// aggregate prefix) rather than dropping the frame, for any future event
// name that doesn't fit the "agentchat.<kind>.<id>" pattern.
func TestEventKind_UnrecognizedShape(t *testing.T) {
	assert.Equal(t, "weird", eventKind("agentchat.weird"))
}
