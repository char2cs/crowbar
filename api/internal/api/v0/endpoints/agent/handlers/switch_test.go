package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestSwitch_Success proves Switch decodes {provider}, calls SwitchProvider
// with the path id and decoded provider, and responds 200 with the mutation
// envelope carrying the new segment's id.
func TestSwitch_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{switchNewSegID: "seg-2"}
	h := handlers.New(uc)

	body := []byte(`{"provider":"vendor-b"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats/chat-1/switch", body)
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Switch(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.True(t, envelope.Success)
	assert.Equal(t, "seg-2", envelope.Data.ID)

	require.Len(t, uc.switchCalls, 1)
	assert.Equal(t, "chat-1", uc.switchCalls[0].chatID)
	assert.Equal(t, "vendor-b", uc.switchCalls[0].provider)
}

// TestSwitch_BadJSON proves a malformed body is rejected 400 without reaching
// the usecase.
func TestSwitch_BadJSON(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats/chat-1/switch", []byte("{not json"))
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Switch(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.switchCalls)
}

// TestSwitch_WrongWorkspace404s proves the by-id scope check
// (requireChatInWorkspace): a chat anchored to a DIFFERENT workspace than the
// :wsId path param 404s before SwitchProvider is ever called (Task 3).
func TestSwitch_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{getChat: domain.AgentChat{ID: "chat-1", WorkspaceID: "ws-other"}}
	h := handlers.New(uc)

	body := []byte(`{"provider":"vendor-b"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/chat-1/switch", body)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "chat-1"}}

	h.Switch(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.switchCalls, "SwitchProvider must never be called once the scope check 404s")
}

// TestSwitch_UsecaseError proves a SwitchProvider failure surfaces as a mapped
// error response via StatusAndMessage rather than a bare 200.
func TestSwitch_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{switchErr: agentchat.ErrNotFound}
	h := handlers.New(uc)

	body := []byte(`{"provider":"vendor-b"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats/missing/switch", body)
	ctx.Params = gin.Params{{Key: "id", Value: "missing"}}

	h.Switch(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestHandoff_Success proves Handoff calls AssembleHandoff for the path id and
// responds 200 with the handoff string under the query envelope.
func TestHandoff_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{handoffStr: "=== HANDED-OFF CONTEXT ==="}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/agent/chats/chat-1/handoff", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Handoff(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Handoff string `json:"handoff"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.True(t, envelope.Success)
	assert.Equal(t, "=== HANDED-OFF CONTEXT ===", envelope.Data.Handoff)
}

// TestHandoff_WrongWorkspace404s proves the by-id scope check
// (requireChatInWorkspace): a chat anchored to a DIFFERENT workspace than the
// :wsId path param 404s before AssembleHandoff is ever called (Task 3).
func TestHandoff_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{getChat: domain.AgentChat{ID: "chat-1", WorkspaceID: "ws-other"}}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/chat-1/handoff", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "chat-1"}}

	h.Handoff(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestHandoff_UsecaseError proves an AssembleHandoff failure surfaces as a
// mapped error response.
func TestHandoff_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{handoffErr: errors.New("boom")}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/agent/chats/chat-1/handoff", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Handoff(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
