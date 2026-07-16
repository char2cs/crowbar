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
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
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
// the List/Get handlers: each test dials in the chats or errors it needs to
// exercise a given branch. SpawnChat/IngestHook are not exercised through this
// double (see fakeAgentUsecase for those).
type configurableListGetUsecase struct {
	chats     []domain.AgentChat
	listErr   error
	listWsIDs []string

	chat   domain.AgentChat
	getErr error

	// liveRunners maps a chat id to the runner PLACED on it. A chat ABSENT from
	// the map is DORMANT, and LiveRunnerForChat answers agentrunner.ErrNotFound —
	// which is the real answer (no live row exists, because no PTY does), not a
	// failure. liveErr is the genuine-failure branch (a read that blew up).
	liveRunners map[string]domain.AgentRunner
	liveErr     error

	// conversations maps a chat id to its append-only history, OLDEST FIRST, so
	// the last element is the chat's last conversation — the provider fallback a
	// dormant chat's activeProviderId derives from.
	conversations map[string][]domain.ChatConversation
	convErr       error
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

func (u *configurableListGetUsecase) LiveRunnerForChat(
	_ context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	if u.liveErr != nil {
		return domain.AgentRunner{}, u.liveErr
	}
	runner, ok := u.liveRunners[chatID]
	if !ok {
		return domain.AgentRunner{}, agentrunner.ErrNotFound
	}
	return runner, nil
}

func (u *configurableListGetUsecase) ConversationsForChat(
	_ context.Context,
	chatID string,
) ([]domain.ChatConversation, error) {
	if u.convErr != nil {
		return nil, u.convErr
	}
	return u.conversations[chatID], nil
}

func (configurableListGetUsecase) SwitchProvider(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (configurableListGetUsecase) ResumeChat(
	_ context.Context,
	_ string,
) (string, error) {
	return "", nil
}

func (configurableListGetUsecase) StopChat(
	_ context.Context,
	_ string,
) error {
	return nil
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

func (configurableListGetUsecase) RenameByRunner(
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

// chatRow mirrors the derived runner facts on the wire shape of dto.AgentChatDTO:
// the runner PLACED on the chat right now and the PTY the pane attaches to, both
// joined on at read time from the live-runner projection, plus the provider the
// switch dropdown shows. There is deliberately no status/isLive field to read: the
// presence of liveRunnerId IS the liveness answer, because the live row exists
// exactly while the process does.
type chatRow struct {
	ID                string `json:"id"`
	LiveRunnerID      string `json:"liveRunnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
	ActiveProviderID  string `json:"activeProviderId"`
}

// TestList_DormantChatFallsBackToLastConversationProvider is the subtle one. A chat
// whose CLI has EXITED has no live runner — so liveRunnerId and terminalSessionId are
// empty, and the frontend needs no second liveness check to know the pane cannot
// attach. But it must still show the right provider glyph and dropdown selection, and
// Resume must know who to bring back, so activeProviderId falls back to the provider
// of the chat's LAST conversation (history is oldest-first, so the last element wins —
// a chat switched vendor-a -> vendor-b answers vendor-b, not vendor-a).
func TestList_DormantChatFallsBackToLastConversationProvider(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.AgentChat{{ID: "c1", WorkspaceID: "ws1"}},
		// No live runner for c1: the chat is dormant.
		conversations: map[string][]domain.ChatConversation{
			"c1": {
				{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1", FirstSeenAt: time.Unix(1, 0).UTC()},
				{ChatID: "c1", ProviderID: "vendor-b", SessionID: "sess-2", FirstSeenAt: time.Unix(2, 0).UTC()},
			},
		},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

	h.List(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data []chatRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	assert.Empty(t, envelope.Data[0].LiveRunnerID, "a dormant chat has no runner: absence IS the liveness answer")
	assert.Empty(t, envelope.Data[0].TerminalSessionID, "no runner, no PTY to attach to")
	assert.Equal(t, "vendor-b", envelope.Data[0].ActiveProviderID, "dormant falls back to the LAST conversation's provider")
}

// TestList_LiveChatCarriesRunnerAndPTY proves the live join: a chat a runner is
// placed on carries that runner's id (the liveness answer) and its terminal session
// (what the pane attaches to), and activeProviderId is the LIVE runner's provider —
// which wins over the history even when the last conversation names another vendor
// (mid-switch, the new runner is already the truth).
func TestList_LiveChatCarriesRunnerAndPTY(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.AgentChat{{ID: "c1", WorkspaceID: "ws1"}},
		liveRunners: map[string]domain.AgentRunner{
			"c1": {
				ID:              "run-1",
				WorkspaceID:     "ws1",
				ProviderID:      "vendor-b",
				TerminalSession: "term-1",
				CurrentChatID:   "c1",
			},
		},
		conversations: map[string][]domain.ChatConversation{
			"c1": {{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1"}},
		},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

	h.List(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data []chatRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "run-1", envelope.Data[0].LiveRunnerID)
	assert.Equal(t, "term-1", envelope.Data[0].TerminalSessionID)
	assert.Equal(t, "vendor-b", envelope.Data[0].ActiveProviderID, "the live runner's provider wins over the history")
}

// TestList_ChatWithNoRunnerEverIsEmpty proves a chat that has NEVER had a runner —
// no live row, no conversation history — reads empty everywhere and does not error.
// "Never ran" and "ran and exited" are both dormant; only the provider fallback tells
// them apart.
func TestList_ChatWithNoRunnerEverIsEmpty(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.AgentChat{{ID: "c1", WorkspaceID: "ws1"}},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

	h.List(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data []chatRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 1)
	assert.Empty(t, envelope.Data[0].LiveRunnerID)
	assert.Empty(t, envelope.Data[0].TerminalSessionID)
	assert.Empty(t, envelope.Data[0].ActiveProviderID)
}

// TestList_LiveRunnerLookupError proves a GENUINE live-runner read failure (not the
// ErrNotFound that merely means "dormant") surfaces as a mapped error.
func TestList_LiveRunnerLookupError(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats:   []domain.AgentChat{{ID: "c1", WorkspaceID: "ws1"}},
		liveErr: errors.New("projection down"),
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

	h.List(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestList_ConversationsLookupError proves a conversation-history read failure
// surfaces as a mapped error rather than a half-derived row.
func TestList_ConversationsLookupError(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats:   []domain.AgentChat{{ID: "c1", WorkspaceID: "ws1"}},
		convErr: errors.New("projection down"),
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

	h.List(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestGet_Success proves Get returns the scoped chat with the same derived runner
// facts List carries, PLUS the chat's conversations — the append-only history that
// replaced the deleted segment list.
func TestGet_Success(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.AgentChat{ID: "c1", WorkspaceID: "ws1", Title: "a title"},
		liveRunners: map[string]domain.AgentRunner{
			"c1": {ID: "run-1", ProviderID: "vendor-a", TerminalSession: "term-1", CurrentChatID: "c1"},
		},
		conversations: map[string][]domain.ChatConversation{
			"c1": {{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1", FirstSeenAt: time.Unix(1, 0).UTC()}},
		},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data struct {
			ID                string `json:"id"`
			WorkspaceID       string `json:"workspaceId"`
			Title             string `json:"title"`
			LiveRunnerID      string `json:"liveRunnerId"`
			TerminalSessionID string `json:"terminalSessionId"`
			ActiveProviderID  string `json:"activeProviderId"`
			Conversations     []struct {
				ProviderID string `json:"providerId"`
				SessionID  string `json:"sessionId"`
			} `json:"conversations"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, "c1", envelope.Data.ID)
	assert.Equal(t, "ws1", envelope.Data.WorkspaceID)
	assert.Equal(t, "a title", envelope.Data.Title)
	assert.Equal(t, "run-1", envelope.Data.LiveRunnerID)
	assert.Equal(t, "term-1", envelope.Data.TerminalSessionID)
	assert.Equal(t, "vendor-a", envelope.Data.ActiveProviderID)

	require.Len(t, envelope.Data.Conversations, 1)
	assert.Equal(t, "vendor-a", envelope.Data.Conversations[0].ProviderID)
	assert.Equal(t, "sess-1", envelope.Data.Conversations[0].SessionID)
}

// TestGet_ConversationsIsNeverNull proves the detail endpoint carries `conversations`
// (the successor of the deleted `segments`) as [] rather than null on a chat that has
// hosted none — the FE maps over it unguarded.
func TestGet_ConversationsIsNeverNull(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.AgentChat{ID: "c1", WorkspaceID: "ws1"},
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"conversations":[]`)
	assert.NotContains(t, rec.Body.String(), `"segments"`, "segments are gone with AgentSegment")
}

// TestGet_RuntimeLookupError proves a failing runner-projection read on the detail
// route surfaces as a mapped error rather than a chat with silently empty runner
// facts — an empty liveRunnerId means DORMANT, and must never mean "the read broke".
func TestGet_RuntimeLookupError(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat:    domain.AgentChat{ID: "c1", WorkspaceID: "ws1"},
		liveErr: errors.New("projection down"),
	}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/agent/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
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

// TestRenameByRunner_PostsTitleAndSource proves RenameByRunner decodes
// {title}, forwards the :segid path param, decoded title, and the `source`
// query param to RenameByRunner, and responds 202 with an empty body on
// success. Unlike Rename it makes NO by-id workspace scope check first: :segid
// names a RUNNER, resolved straight off the runner aggregate, exactly like the
// Hooks route.
func TestRenameByRunner_PostsTitleAndSource(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	body := []byte(`{"title":"New Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/runners/seg-1/rename?source=agent", body)
	ctx.Params = gin.Params{{Key: "segid", Value: "seg-1"}}

	h.RenameByRunner(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())

	require.Len(t, uc.renameByRunnerCalls, 1)
	assert.Equal(t, "seg-1", uc.renameByRunnerCalls[0].runnerID)
	assert.Equal(t, "New Title", uc.renameByRunnerCalls[0].title)
	assert.Equal(t, "agent", uc.renameByRunnerCalls[0].source)
}

// TestRenameByRunner_BadJSON proves a malformed body is rejected 400 without
// reaching the usecase.
func TestRenameByRunner_BadJSON(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/runners/seg-1/rename", []byte("{not json"))
	ctx.Params = gin.Params{{Key: "segid", Value: "seg-1"}}

	h.RenameByRunner(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.renameByRunnerCalls)
}

// TestRenameByRunner_UnknownRunner404s proves a RenameByRunner failure
// (agentrunner.ErrNotFound — an unknown or already-exited runner) surfaces as
// a mapped 404 rather than a 202.
func TestRenameByRunner_UnknownRunner404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{renameByRunnerErr: agentrunner.ErrNotFound}
	h := handlers.New(uc)

	body := []byte(`{"title":"New Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/runners/missing/rename", body)
	ctx.Params = gin.Params{{Key: "segid", Value: "missing"}}

	h.RenameByRunner(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
