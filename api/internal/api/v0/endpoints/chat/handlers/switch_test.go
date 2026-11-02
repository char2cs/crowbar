package handlers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// TestSwitch_Success proves Switch decodes {provider}, calls SwitchProvider
// with the path id and decoded provider, and responds 200 with the mutation
// envelope carrying the new segment's id.
func TestSwitch_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{switchNewSegID: "seg-2"}
	h := newChatHandlers(uc)

	body := []byte(`{"provider":"vendor-b"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/chat-1/switch", body)
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

// A switch refused because the chat is working carries the busy code, so a
// client tells it apart from every other conflict.
func TestSwitch_ABusyChatIsRefusedWithTheBusyCode(t *testing.T) {
	uc := &fakeAgentUsecase{switchErr: agentusecase.ErrPromptBusy}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/chat-1/switch", []byte(`{"provider":"vendor-b"}`))
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Switch(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
	var envelope struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, string(problem.ChatBusy), envelope.Code)
}

// TestHandoff_Success proves Handoff calls AssembleHandoff for the path id and
// responds 200 with the handoff string under the query envelope.
func TestHandoff_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{handoffStr: "=== HANDED-OFF CONTEXT ==="}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/chats/chat-1/handoff", nil)
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

// TestResume_Success proves Resume calls ResumeChat for the path id and responds
// 200 with the (re)active segment id under the mutation envelope.
func TestResume_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{resumeSegID: "seg-9"}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/chat-1/resume", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Resume(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.True(t, envelope.Success)
	assert.Equal(t, "seg-9", envelope.Data.ID)

	require.Len(t, uc.resumeCalls, 1)
	assert.Equal(t, "chat-1", uc.resumeCalls[0])
}

// TestStop_Success proves Stop calls StopChat for the path id and responds 204
// (accepted, no body) — the chat is dormant-ized, not deleted.
func TestStop_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/chat-1/stop", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Stop(ctx)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, uc.stopCalls, 1)
	assert.Equal(t, "chat-1", uc.stopCalls[0])
}

// TestClose_Success proves Close calls CloseChat (not StopChat) for the path id
// and responds 204.
func TestClose_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/chat-1/close", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "chat-1"}}

	h.Close(ctx)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"chat-1"}, uc.closeCalls)
	assert.Empty(t, uc.stopCalls)
}

// A by-id route 404s a chat anchored to another workspace than :wsId exactly
// like an unknown id, before its usecase (or the tree behind it) is reached.
func TestByIDRoutes_AChatOfAnotherWorkspaceIs404BeforeTheUsecase(t *testing.T) {
	for _, tc := range []struct {
		name    string
		route   func(*handlers.Handlers, *gin.Context)
		body    string
		reached func(*fakeAgentUsecase, *fakeChatTree) int
	}{
		{"switch", (*handlers.Handlers).Switch, `{"provider":"vendor-b"}`,
			func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.switchCalls) }},
		{"handoff", (*handlers.Handlers).Handoff, "", nil},
		{"resume", (*handlers.Handlers).Resume, "",
			func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.resumeCalls) }},
		{"stop", (*handlers.Handlers).Stop, "",
			func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.stopCalls) }},
		{"rename", (*handlers.Handlers).Rename, `{"title":"My Title"}`,
			func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.renameCalls) }},
		{"set selection", (*handlers.Handlers).SetSelection, `{"model":"opus"}`,
			func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.selectionCalls) }},
		{"delete", (*handlers.Handlers).Delete, "",
			func(uc *fakeAgentUsecase, tree *fakeChatTree) int { return len(uc.purgeCalls) + len(tree.gotPurge) }},
		{"get", (*handlers.Handlers).Get, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c-1", WorkspaceID: "ws-other"}}
			tree := &fakeChatTree{}
			ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c-1", []byte(tc.body))
			ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c-1"}}

			tc.route(newFolderHandlersWith(uc, tree), ctx)

			assert.Equal(t, http.StatusNotFound, rec.Code)
			if tc.reached != nil {
				assert.Zero(t, tc.reached(uc, tree), "the scope check must 404 before the usecase is reached")
			}
		})
	}
}

// A body that does not decode is 400 before the usecase is reached, and a
// usecase failure surfaces through the shared mapper with its own status,
// never a blanket 500 or a bare success.
func TestRoutes_RefuseBadBodiesAndMapUsecaseFailures(t *testing.T) {
	const rpc = `{"token":"TOK","rpc":{"jsonrpc":"2.0","id":1,"method":"ping"}}`
	byID := gin.Params{{Key: "id", Value: "c-1"}}
	runner := gin.Params{{Key: "segid", Value: "seg-1"}}
	for _, tc := range []struct {
		name    string
		route   func(*handlers.Handlers, *gin.Context)
		uc      fakeAgentUsecase
		list    *configurableListGetUsecase
		treeErr error
		params  gin.Params
		body    string
		code    int
		reached func(*fakeAgentUsecase) int
	}{
		{name: "switch: bad JSON", route: (*handlers.Handlers).Switch, params: byID, body: "{not json",
			code: http.StatusBadRequest, reached: func(uc *fakeAgentUsecase) int { return len(uc.switchCalls) }},
		{name: "rename: bad JSON", route: (*handlers.Handlers).Rename, params: byID, body: "{not json",
			code: http.StatusBadRequest, reached: func(uc *fakeAgentUsecase) int { return len(uc.renameCalls) }},
		{name: "set selection: bad JSON", route: (*handlers.Handlers).SetSelection, params: byID, body: "{not json",
			code: http.StatusBadRequest, reached: func(uc *fakeAgentUsecase) int { return len(uc.selectionCalls) }},
		{name: "mcp: bad JSON", route: (*handlers.Handlers).MCP, params: runner, body: "{not json",
			code: http.StatusBadRequest, reached: func(uc *fakeAgentUsecase) int { return len(uc.dispatchMCPCalls) }},
		// An empty message is not a notification: answering it 204 would hide the caller's bug.
		{name: "mcp: no rpc member", route: (*handlers.Handlers).MCP, params: runner, body: `{"token":"TOK"}`,
			code: http.StatusBadRequest, reached: func(uc *fakeAgentUsecase) int { return len(uc.dispatchMCPCalls) }},
		{name: "switch: not found", route: (*handlers.Handlers).Switch, uc: fakeAgentUsecase{switchErr: agentchat.ErrNotFound},
			params: byID, body: `{"provider":"vendor-b"}`, code: http.StatusNotFound},
		{name: "handoff: unmapped", route: (*handlers.Handlers).Handoff, uc: fakeAgentUsecase{handoffErr: errors.New("boom")},
			params: byID, code: http.StatusInternalServerError},
		{name: "resume: not found", route: (*handlers.Handlers).Resume, uc: fakeAgentUsecase{resumeErr: agentchat.ErrNotFound},
			params: byID, code: http.StatusNotFound},
		{name: "stop: not found", route: (*handlers.Handlers).Stop, uc: fakeAgentUsecase{stopErr: agentchat.ErrNotFound},
			params: byID, code: http.StatusNotFound},
		{name: "rename: not found", route: (*handlers.Handlers).Rename, uc: fakeAgentUsecase{renameErr: agentchat.ErrNotFound},
			params: byID, body: `{"title":"My Title"}`, code: http.StatusNotFound},
		// A value outside the provider's catalogue is the client's error, not an opaque 500.
		{name: "set selection: rejected value", route: (*handlers.Handlers).SetSelection,
			uc:     fakeAgentUsecase{selectionErr: fmt.Errorf("no such model: %w", problem.ErrInvalidArgument)},
			params: byID, body: `{"model":"telepathy"}`, code: http.StatusBadRequest},
		{name: "delete: not found", route: (*handlers.Handlers).Delete, treeErr: agentchat.ErrNotFound,
			params: byID, code: http.StatusNotFound},
		{name: "delete: a working subtree", route: (*handlers.Handlers).Delete, treeErr: agentusecase.ErrTreeSubtreeWorking,
			params: byID, code: http.StatusConflict},
		{name: "mcp: mapped", route: (*handlers.Handlers).MCP, uc: fakeAgentUsecase{dispatchMCPErr: agentusecase.ErrNoLiveRunner},
			params: runner, body: rpc, code: http.StatusNotFound},
		{name: "mcp: unmapped", route: (*handlers.Handlers).MCP, uc: fakeAgentUsecase{dispatchMCPErr: errors.New("tool surface not configured")},
			params: runner, body: rpc, code: http.StatusInternalServerError},
		{name: "get: not found", route: (*handlers.Handlers).Get, list: &configurableListGetUsecase{getErr: agentchat.ErrNotFound},
			params: byID, code: http.StatusNotFound},
		{name: "list: by workspace", route: (*handlers.Handlers).List, list: &configurableListGetUsecase{listErr: errors.New("db down")},
			params: gin.Params{{Key: "wsId", Value: "ws1"}}, code: http.StatusInternalServerError},
		{name: "list: by repo", route: (*handlers.Handlers).List, list: &configurableListGetUsecase{listErr: errors.New("db down")},
			params: gin.Params{{Key: "repoId", Value: "r1"}}, code: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc := &tc.uc
			var ports agentUsecases = uc
			if tc.list != nil {
				ports = tc.list
			}
			ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/c-1", []byte(tc.body))
			ctx.Params = tc.params

			tc.route(newFolderHandlersWith(ports, &fakeChatTree{err: tc.treeErr}), ctx)

			assert.Equal(t, tc.code, rec.Code)
			if tc.reached != nil {
				assert.Zero(t, tc.reached(uc))
			}
		})
	}
}
