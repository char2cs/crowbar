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
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// TestCreate_Success proves Create reads the workspace id from the :wsId
// path param (Task 3: nested under .../workspaces/:wsId/agent/chats) and
// provider from the body, calls SpawnChat with both, and responds 201 with
// the mutation envelope carrying the new chat's id.
func TestCreate_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{spawnChatID: "chat-1", spawnSegID: "seg-1"}
	h := handlers.New(uc)

	body := []byte(`{"provider":"vendor-a"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws-1/agent/chats", body)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

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

	require.Len(t, uc.spawnCalls, 1)
	assert.Equal(t, "ws-1", uc.spawnCalls[0].workspaceID)
	assert.Equal(t, "vendor-a", uc.spawnCalls[0].provider)
}

// TestCreate_BadJSON proves a malformed body is rejected 400 without reaching
// the usecase.
func TestCreate_BadJSON(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws-1/agent/chats", []byte("{not json"))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Create(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.spawnCalls)
}

// TestCreate_UsecaseError proves a SpawnChat failure surfaces as a mapped
// error response.
func TestCreate_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{spawnErr: errors.New("boom")}
	h := handlers.New(uc)

	body := []byte(`{"provider":"vendor-a"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws-1/agent/chats", body)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Create(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// configurableListGetUsecase is a configurable AgentUsecase double dedicated to
// the List/Get handlers: each test dials in the chats/segments or errors it
// needs to exercise a given branch. SpawnChat/IngestHook are not exercised
// through this double (see fakeAgentUsecase for those).
type configurableListGetUsecase struct {
	chats     []domain.AgentChat
	listErr   error
	listWsIDs []string

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

func (u *configurableListGetUsecase) ListChatsByWorkspace(
	_ context.Context,
	wsID string,
) ([]domain.AgentChat, error) {
	u.listWsIDs = append(u.listWsIDs, wsID)
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

func (configurableListGetUsecase) RenameChat(
	_ context.Context,
	_, _, _ string,
) error {
	return nil
}

func (configurableListGetUsecase) PurgeChat(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (configurableListGetUsecase) ListProviders(
	_ context.Context,
	_ string,
) ([]engineagent.Descriptor, error) {
	return nil, nil
}

// TestList_Success proves List reads the :wsId path param (Task 3: nested
// under .../workspaces/:wsId/agent/chats), forwards it to
// ListChatsByWorkspace, and returns every chat as wire DTOs.
func TestList_Success(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.AgentChat{
			{ID: "c1", WorkspaceID: "ws1", CreatedAt: time.Unix(1, 0).UTC()},
			{ID: "c2", WorkspaceID: "ws1", CreatedAt: time.Unix(2, 0).UTC()},
		},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

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

	require.Equal(t, []string{"ws1"}, uc.listWsIDs)
}

// TestList_UsecaseError proves a ListChatsByWorkspace failure surfaces as a
// mapped error.
func TestList_UsecaseError(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{listErr: errors.New("db down")}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

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
			{ID: "seg-1", ProviderID: "vendor-a"},
			{ID: "seg-2", ProviderID: "vendor-a"},
		},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c1"}}

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

// TestGet_WrongWorkspace404s proves the by-id scope check
// (requireChatInWorkspace): a chat that exists but is anchored to a
// DIFFERENT workspace than the :wsId path param 404s exactly like an unknown
// id, never leaking that the chat exists elsewhere (Task 3).
func TestGet_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.AgentChat{ID: "c1", WorkspaceID: "ws-other"},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
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

// TestRename_PostsTitleAndSource proves Rename decodes {title}, forwards the
// path id, decoded title, and the `source` query param to RenameChat, and
// responds 202 with an empty body on success.
func TestRename_PostsTitleAndSource(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	body := []byte(`{"title":"My Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats/c-1/rename?source=agent", body)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Rename(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())

	require.Len(t, uc.renameCalls, 1)
	assert.Equal(t, "c-1", uc.renameCalls[0].chatID)
	assert.Equal(t, "My Title", uc.renameCalls[0].title)
	assert.Equal(t, "agent", uc.renameCalls[0].source)
}

// TestRename_WrongWorkspace404s proves the by-id scope check
// (requireChatInWorkspace): a chat anchored to a DIFFERENT workspace than the
// :wsId path param 404s before RenameChat is ever called (Task 3).
func TestRename_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{getChat: domain.AgentChat{ID: "c-1", WorkspaceID: "ws-other"}}
	h := handlers.New(uc)

	body := []byte(`{"title":"My Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/c-1/rename", body)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c-1"}}

	h.Rename(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.renameCalls, "RenameChat must never be called once the scope check 404s")
}

// TestRename_BadJSON proves a malformed body is rejected 400 without reaching
// the usecase.
func TestRename_BadJSON(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats/c-1/rename", []byte("{not json"))
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Rename(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.renameCalls)
}

// TestRename_UsecaseError proves a RenameChat failure surfaces as a mapped
// error response rather than a 202.
func TestRename_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{renameErr: agentchat.ErrNotFound}
	h := handlers.New(uc)

	body := []byte(`{"title":"My Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/chats/missing/rename", body)
	ctx.Params = gin.Params{{Key: "id", Value: "missing"}}

	h.Rename(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestDelete_Success proves Delete forwards the path id to PurgeChat and
// responds 202 with an empty body on success (Task 5).
func TestDelete_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/agent/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	assert.Equal(t, []string{"c-1"}, uc.purgeCalls)
}

// TestDelete_WrongWorkspace404s proves the by-id scope check
// (requireChatInWorkspace): a chat anchored to a DIFFERENT workspace than the
// :wsId path param 404s before PurgeChat is ever called (Task 5).
func TestDelete_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{getChat: domain.AgentChat{ID: "c-1", WorkspaceID: "ws-other"}}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.purgeCalls, "PurgeChat must never be called once the scope check 404s")
}

// TestDelete_UsecaseError proves a PurgeChat failure surfaces as a mapped
// error response rather than a 202.
func TestDelete_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{purgeErr: agentchat.ErrNotFound}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/agent/chats/missing", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "missing"}}

	h.Delete(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
