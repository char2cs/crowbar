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

type frame struct {
	chatID      string
	workspaceID string
	forgotten   bool
}

type captureHub struct {
	mu     sync.Mutex
	frames []frame
}

func (h *captureHub) push(chatID, workspaceID string, forgotten bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = append(h.frames, frame{
		chatID:      chatID,
		workspaceID: workspaceID,
		forgotten:   forgotten,
	})
}

func (h *captureHub) watch(e ChatEvent) {
	h.push(e.ChatID, e.WorkspaceID, e.Forgotten)
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

func TestHubProjection_Create_BroadcastsCreatedFrame(t *testing.T) {
	ctx, ax, _, h := newProjected(t)
	_, err := ax.SendWait(ctx, createCmd("c1"))
	require.NoError(t, err)

	frames := h.all()
	require.Len(t, frames, 1)
	assert.Equal(t, frame{chatID: "c1", workspaceID: "w1"}, frames[0])
}

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
	assert.Equal(t, frame{chatID: "c1", workspaceID: "ws-9"}, frames[0])
	assert.Equal(t, frame{chatID: "c1", workspaceID: "ws-9", forgotten: true}, frames[1])
}

func TestRegisterHubProjection_SubscribeError(t *testing.T) {
	err := registerHubProjection(&fakeAx{subscribeErr: errors.New("bus down")}, func(ChatEvent) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat hub projection: subscribe")
}

func TestRegisterHubProjection_OnForgetError(t *testing.T) {
	err := registerHubProjection(&fakeAx{forgetErr: errors.New("bus down")}, func(ChatEvent) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agentchat hub projection: onforget")
}

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
		ChatID: "chat-1", WorkspaceID: "ws-1",
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
		ChatID: "chat-9", WorkspaceID: "ws-1", Forgotten: true,
		Chat: domain.Chat{ID: "chat-9", WorkspaceID: "ws-1"},
	}, got[0])
}

func TestHubProjector_NilWatch_DoesNotPanic(t *testing.T) {
	p := &hubProjector{}
	p.onEvent(context.Background(), asynxModels.Event[domain.Chat]{
		AggregateID: "chat-1",
		EventName:   "agentchat.created.chat-1",
	})
	p.onForget(asynxModels.Event[domain.Chat]{})
}
