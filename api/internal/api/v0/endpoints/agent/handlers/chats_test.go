package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestCreate_Success proves Create decodes {workspaceId, provider}, calls
// SpawnChat, and responds 201 with the mutation envelope carrying the new
// chat's id.
func TestCreate_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{spawnChatID: "chat-1", spawnSegID: "seg-1"}
	h := handlers.New(uc)

	body := []byte(`{"workspaceId":"ws-1","provider":"vendor-a"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats", body)

	h.Create(ctx)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.True(t, envelope.Success)
	assert.Equal(t, "chat-1", envelope.Data.ID)
}

// TestCreate_BadJSON proves a malformed body is rejected 400 without reaching
// the usecase.
func TestCreate_BadJSON(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats", []byte("{not json"))

	h.Create(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestCreate_UsecaseError proves a SpawnChat failure surfaces as a mapped
// error response.
func TestCreate_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{spawnErr: errors.New("boom")}
	h := handlers.New(uc)

	body := []byte(`{"workspaceId":"ws-1","provider":"vendor-a"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats", body)

	h.Create(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// configurableListGetUsecase is a configurable AgentUsecase double dedicated to
// the List/Get handlers: each test dials in the chats/segments or errors it
// needs to exercise a given branch. SpawnChat/IngestHook are not exercised
// through this double (see fakeAgentUsecase for those).
type configurableListGetUsecase struct {
	chats   []domain.AgentChat
	listErr error

	chat    domain.AgentChat
	getErr  error
	segs    []domain.AgentSegment
	segsErr error
}

func (configurableListGetUsecase) SpawnChat(
	_ context.Context,
	_ string,
	_ string,
) (string, string, error) {
	return "", "", nil
}

func (configurableListGetUsecase) IngestHook(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ []byte,
) error {
	return nil
}

func (u *configurableListGetUsecase) ListChats(
	_ context.Context,
) ([]domain.AgentChat, error) {
	if u.listErr != nil {
		return nil, u.listErr
	}
	return u.chats, nil
}

func (u *configurableListGetUsecase) GetChat(
	_ context.Context,
	_ string,
) (domain.AgentChat, error) {
	if u.getErr != nil {
		return domain.AgentChat{}, u.getErr
	}
	return u.chat, nil
}

func (u *configurableListGetUsecase) SegmentsFor(
	_ context.Context,
	_ string,
) ([]domain.AgentSegment, error) {
	if u.segsErr != nil {
		return nil, u.segsErr
	}
	return u.segs, nil
}

func (configurableListGetUsecase) SwitchProvider(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (configurableListGetUsecase) AssembleHandoff(
	_ context.Context,
	_ string,
) (string, error) {
	return "", nil
}

// TestList_Success proves List returns every chat as wire DTOs.
func TestList_Success(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.AgentChat{
			{ID: "c1", WorkspaceID: "ws1", CreatedAt: time.Unix(1, 0).UTC()},
			{ID: "c2", WorkspaceID: "ws2", CreatedAt: time.Unix(2, 0).UTC()},
		},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/agent/chats", nil)

	h.List(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 2)
	assert.Equal(t, "c1", envelope.Data[0].ID)
}

// TestList_UsecaseError proves a ListChats failure surfaces as a mapped error.
func TestList_UsecaseError(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{listErr: errors.New("db down")}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/agent/chats", nil)

	h.List(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestGet_Success proves Get composes the chat with its ordered segment
// history into the detail wire shape.
func TestGet_Success(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.AgentChat{ID: "c1", WorkspaceID: "ws1", ActiveSegmentID: "seg-2"},
		segs: []domain.AgentSegment{
			{ID: "seg-1", ChatID: "c1", ProviderID: "vendor-a"},
			{ID: "seg-2", ChatID: "c1", ProviderID: "vendor-a"},
		},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/agent/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data struct {
			ID       string `json:"id"`
			Segments []struct {
				ID string `json:"id"`
			} `json:"segments"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, "c1", envelope.Data.ID)
	require.Len(t, envelope.Data.Segments, 2)
	assert.Equal(t, "seg-1", envelope.Data.Segments[0].ID)
}

// TestGet_ChatNotFound proves an unknown chat id 404s via the
// agentchat.ErrNotFound -> StatusAndMessage mapping.
func TestGet_ChatNotFound(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{getErr: agentchat.ErrNotFound}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/agent/chats/missing", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "missing"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGet_SegmentsError proves a SegmentsFor failure surfaces as a mapped
// error even when GetChat succeeds.
func TestGet_SegmentsError(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat:    domain.AgentChat{ID: "c1"},
		segsErr: errors.New("boom"),
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/agent/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
