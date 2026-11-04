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

// TestKill_Success proves Kill calls KillChat (not StopChat) for the path id
// and responds 204.
func TestKill_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/chat-1/kill", nil)
	ctx.Params = gin.Params{{Key: "chatId", Value: "chat-1"}}

	h.Kill(ctx)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"chat-1"}, uc.killCalls)
	assert.Empty(t, uc.stopCalls)
}

// A by-id route 404s a chat anchored to another workspace than :wsId exactly
// like an unknown id, before its usecase (or the tree behind it) is reached.
func TestByIDRoutes_AChatOfAnotherWorkspaceIs404BeforeTheUsecase(t *testing.T) {
	type calls = func(*fakeAgentUsecase, *fakeChatTree) int
	switched := func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.switchCalls) }
	resumed := func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.resumeCalls) }
	stopped := func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.stopCalls) }
	renamed := func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.renameCalls) }
	selected := func(uc *fakeAgentUsecase, _ *fakeChatTree) int { return len(uc.selectionCalls) }
	purged := func(uc *fakeAgentUsecase, tree *fakeChatTree) int { return len(uc.purgeCalls) + len(tree.gotPurge) }
	for _, tc := range []struct {
		name    string
		route   func(*handlers.Handlers, *gin.Context)
		body    string
		reached calls
	}{
		{"switch", (*handlers.Handlers).Switch, `{"provider":"vendor-b"}`, switched},
		{"handoff", (*handlers.Handlers).Handoff, "", nil},
		{"resume", (*handlers.Handlers).Resume, "", resumed},
		{"stop", (*handlers.Handlers).Stop, "", stopped},
		{"rename", (*handlers.Handlers).Rename, `{"title":"My Title"}`, renamed},
		{"set selection", (*handlers.Handlers).SetSelection, `{"model":"opus"}`, selected},
		{"delete", (*handlers.Handlers).Delete, "", purged},
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

// A body that does not decode is 400 before the usecase is reached; a usecase
// failure surfaces through the shared mapper with its own status, never a
// blanket 500 or a bare success. An empty MCP message is not a notification,
// and a selection outside the provider's catalogue is the client's error.
func TestRoutes_RefuseBadBodiesAndMapUsecaseFailures(t *testing.T) {
	const rpc = `{"token":"TOK","rpc":{"jsonrpc":"2.0","id":1,"method":"ping"}}`
	type h = handlers.Handlers
	byID, runner := gin.Params{{Key: "id", Value: "c-1"}}, gin.Params{{Key: "segid", Value: "seg-1"}}
	byWS, byRepo := gin.Params{{Key: "wsId", Value: "ws1"}}, gin.Params{{Key: "repoId", Value: "r1"}}
	switched := func(uc *fakeAgentUsecase) int { return len(uc.switchCalls) }
	renamed := func(uc *fakeAgentUsecase) int { return len(uc.renameCalls) }
	selected := func(uc *fakeAgentUsecase) int { return len(uc.selectionCalls) }
	dispatched := func(uc *fakeAgentUsecase) int { return len(uc.dispatchMCPCalls) }
	invalid := fmt.Errorf("no such model: %w", problem.ErrInvalidArgument)
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
		{"switch: bad JSON", (*h).Switch, fakeAgentUsecase{}, nil, nil, byID, "{not json", http.StatusBadRequest, switched},
		{"rename: bad JSON", (*h).Rename, fakeAgentUsecase{}, nil, nil, byID, "{not json", http.StatusBadRequest, renamed},
		{"set selection: bad JSON", (*h).SetSelection, fakeAgentUsecase{}, nil, nil, byID, "{not json", http.StatusBadRequest, selected},
		{"mcp: bad JSON", (*h).MCP, fakeAgentUsecase{}, nil, nil, runner, "{not json", http.StatusBadRequest, dispatched},
		{"mcp: no rpc member", (*h).MCP, fakeAgentUsecase{}, nil, nil, runner, `{"token":"TOK"}`, http.StatusBadRequest, dispatched},
		{"switch: not found", (*h).Switch, fakeAgentUsecase{switchErr: agentchat.ErrNotFound}, nil, nil, byID, `{"provider":"vendor-b"}`, http.StatusNotFound, nil},
		{"handoff: unmapped", (*h).Handoff, fakeAgentUsecase{handoffErr: errors.New("boom")}, nil, nil, byID, "", http.StatusInternalServerError, nil},
		{"resume: not found", (*h).Resume, fakeAgentUsecase{resumeErr: agentchat.ErrNotFound}, nil, nil, byID, "", http.StatusNotFound, nil},
		{"stop: not found", (*h).Stop, fakeAgentUsecase{stopErr: agentchat.ErrNotFound}, nil, nil, byID, "", http.StatusNotFound, nil},
		{"rename: not found", (*h).Rename, fakeAgentUsecase{renameErr: agentchat.ErrNotFound}, nil, nil, byID, `{"title":"My Title"}`, http.StatusNotFound, nil},
		{"set selection: rejected value", (*h).SetSelection, fakeAgentUsecase{selectionErr: invalid}, nil, nil, byID, `{"model":"telepathy"}`, http.StatusBadRequest, nil},
		{"delete: not found", (*h).Delete, fakeAgentUsecase{}, nil, agentchat.ErrNotFound, byID, "", http.StatusNotFound, nil},
		{"delete: a working subtree", (*h).Delete, fakeAgentUsecase{}, nil, agentusecase.ErrTreeSubtreeWorking, byID, "", http.StatusConflict, nil},
		{"mcp: mapped", (*h).MCP, fakeAgentUsecase{dispatchMCPErr: agentusecase.ErrNoLiveRunner}, nil, nil, runner, rpc, http.StatusNotFound, nil},
		{"mcp: unmapped", (*h).MCP, fakeAgentUsecase{dispatchMCPErr: errors.New("tool surface not configured")}, nil, nil, runner, rpc, http.StatusInternalServerError, nil},
		{"get: not found", (*h).Get, fakeAgentUsecase{}, &configurableListGetUsecase{getErr: agentchat.ErrNotFound}, nil, byID, "", http.StatusNotFound, nil},
		{"list: by workspace", (*h).List, fakeAgentUsecase{}, &configurableListGetUsecase{listErr: errors.New("db down")}, nil, byWS, "", http.StatusInternalServerError, nil},
		{"list: by repo", (*h).List, fakeAgentUsecase{}, &configurableListGetUsecase{listErr: errors.New("db down")}, nil, byRepo, "", http.StatusInternalServerError, nil},
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
