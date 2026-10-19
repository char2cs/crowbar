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

	accmds "github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// frame is a captured hub broadcast: the (chatID, workspaceID, kind)
// lifecycle triple the hub projection hands to a BroadcastFunc.
type frame struct {
	chatID      string
	workspaceID string
	kind        string
}

// captureHub is a WatchFunc double that records every announcement it receives as the
// frame the fanout would build from it, safe for concurrent pushes from asynx's
// per-shard event-processing goroutines.
type captureHub struct {
	mu     sync.Mutex
	frames []frame
}

func (h *captureHub) push(chatID, workspaceID, kind string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = append(h.frames, frame{
		chatID:      chatID,
		workspaceID: workspaceID,
		kind:        kind,
	})
}

// watch is the seam the store is now wired with. It reduces a ChatEvent to the same
// frame the assertions below have always used, so this change is proven against the
// existing suite rather than a new one.
func (h *captureHub) watch(e ChatEvent) {
	h.push(e.ChatID, e.WorkspaceID, e.Kind)
}

func (h *captureHub) all() []frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]frame(nil), h.frames...)
}

func createCmd(chatID string) accmds.Create {
	return accmds.Create{
		ID: chatID, WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC(),
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
	err := registerHubProjection(&fakeAx{subscribeErr: errors.New("bus down")}, func(ChatEvent) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat hub projection: subscribe")
}

// TestRegisterHubProjection_OnForgetError proves an OnForget subscribe
// failure surfaces wrapped, mirroring registerStoreProjection's own
// OnForget-error test.
func TestRegisterHubProjection_OnForgetError(t *testing.T) {
	err := registerHubProjection(&fakeAx{forgetErr: errors.New("bus down")}, func(ChatEvent) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat hub projection: onforget")
}

// TestHubProjector_OnEvent_DerivesKindFromEventName unit-tests eventKind's
// extraction directly against the hub projector, independent of asynx wiring,
// and proves the workspace id is read off evt.Aggregate.WorkspaceID (not the
// event name/id) and passed through untouched.
func TestHubProjector_OnEvent_DerivesKindFromEventName(t *testing.T) {
	var got frame
	p := &hubProjector{watch: func(e ChatEvent) {
		got = frame{chatID: e.ChatID, workspaceID: e.WorkspaceID, kind: e.Kind}
	}}
	p.onEvent(context.Background(), asynxModels.Event[domain.Chat]{
		AggregateID: "c9",
		EventName:   "agentchat.segment_opened.c9",
		Aggregate:   domain.Chat{WorkspaceID: "w9"},
	})
	assert.Equal(t, frame{chatID: "c9", workspaceID: "w9", kind: "segment_opened"}, got)
}

// TestEventKind_UnrecognizedShape falls back to the name (minus the
// aggregate prefix) rather than dropping the frame, for any future event
// name that doesn't fit the "agentchat.<kind>.<id>" pattern.
func TestEventKind_UnrecognizedShape(t *testing.T) {
	assert.Equal(t, "weird", eventKind("agentchat.weird"))
}

// --- the watch seam itself -------------------------------------------------

func TestHubProjector_OnEvent_EmitsRepoOwnedChatEvent(t *testing.T) {
	var got []ChatEvent
	p := &hubProjector{watch: func(e ChatEvent) { got = append(got, e) }}

	p.onEvent(context.Background(), asynxModels.Event[domain.Chat]{
		AggregateID: "chat-1",
		EventName:   "agentchat.title_set.chat-1",
		Aggregate:   domain.Chat{ID: "chat-1", WorkspaceID: "ws-1", Title: "t"},
	})

	require.Len(t, got, 1)
	assert.Equal(t, ChatEvent{
		ChatID: "chat-1", WorkspaceID: "ws-1", Kind: "title_set",
		Chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1", Title: "t"},
	}, got[0])
}

func TestHubProjector_OnForget_MarksForgotten(t *testing.T) {
	var got []ChatEvent
	p := &hubProjector{watch: func(e ChatEvent) { got = append(got, e) }}

	p.onForget(asynxModels.Event[domain.Chat]{
		Aggregate: domain.Chat{ID: "chat-9", WorkspaceID: "ws-1"},
	})

	require.Len(t, got, 1)
	assert.Equal(t, ChatEvent{
		ChatID: "chat-9", WorkspaceID: "ws-1", Kind: "deleted", Forgotten: true,
		Chat: domain.Chat{ID: "chat-9", WorkspaceID: "ws-1"},
	}, got[0])
}

// A store built without a watch seam must not panic: most unit tests wire none.
func TestHubProjector_NilWatch_DoesNotPanic(t *testing.T) {
	p := &hubProjector{}
	p.onEvent(context.Background(), asynxModels.Event[domain.Chat]{
		AggregateID: "chat-1",
		EventName:   "agentchat.created.chat-1",
	})
	p.onForget(asynxModels.Event[domain.Chat]{})
}
