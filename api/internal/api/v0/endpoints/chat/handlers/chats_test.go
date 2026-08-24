package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// TestCreate_Success proves Create reads the workspace id from the :wsId path
// param (Task 3: nested under .../workspaces/:wsId/chats) and the provider
// from the body, forwards both to the create, and responds 201 with the mutation
// envelope carrying the new chat's id.
//
// A body with no parentId names the panel root, and the handler must forward that
// as the empty string rather than inventing anything — that is the path every
// plain new chat still takes.
func TestCreate_Success(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	h := newChatHandlersWith(&fakeAgentUsecase{}, tree)

	body := []byte(`{"provider":"vendor-a"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws-1/chats", body)
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

	assert.Equal(t, createChatCall{WorkspaceID: "ws-1", ProviderID: "vendor-a"}, tree.gotCreate2)
}

// The parent is the half of a create that decides whether the new chat is a
// THREAD, so it has to reach the usecase intact. A handler that dropped it would
// still create a chat, still answer 201, and produce a chat at the panel root that
// the user asked to be a thread — the failure this asserts against.
func TestCreate_ForwardsTheParentTheChatIsBornUnder(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	h := newChatHandlersWith(&fakeAgentUsecase{}, tree)

	body := []byte(`{"provider":"vendor-a","parentId":"parent-chat"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws-1/chats", body)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Create(ctx)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t,
		createChatCall{WorkspaceID: "ws-1", ProviderID: "vendor-a", ParentID: "parent-chat"},
		tree.gotCreate2)
}

// TestCreate_BadJSON proves a malformed body is rejected 400 without reaching
// the usecase.
func TestCreate_BadJSON(
	t *testing.T,
) {
	tree := &fakeChatTree{}
	h := newChatHandlersWith(&fakeAgentUsecase{}, tree)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws-1/chats", []byte("{not json"))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Create(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, createChatCall{}, tree.gotCreate2)
}

// TestCreate_UsecaseError proves a create failure surfaces as a mapped error
// response.
func TestCreate_UsecaseError(
	t *testing.T,
) {
	tree := &fakeChatTree{err: errors.New("boom")}
	h := newChatHandlersWith(&fakeAgentUsecase{}, tree)

	body := []byte(`{"provider":"vendor-a"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws-1/chats", body)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Create(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// configurableListGetUsecase is a configurable agent-port double dedicated to
// the List/Get handlers: each test dials in the chats or errors it needs to
// exercise a given branch. SpawnChat/IngestHook are not exercised through this
// double (see fakeAgentUsecase for those).
type configurableListGetUsecase struct {
	chats     []domain.Chat
	listErr   error
	listWsIDs []string

	chat   domain.Chat
	getErr error

	// liveRunners maps a chat id to the runner PLACED on it. A chat ABSENT from
	// the map is DORMANT, and LiveRunnerForChat answers agentrunner.ErrNotFound —
	// which is the real answer (no live row exists, because no PTY does), not a
	// failure. liveErr is the genuine-failure branch (a read that blew up).
	liveRunners map[string]engineagents.Runner
	liveErr     error

	// conversations maps a chat id to its append-only history, OLDEST FIRST, so
	// the last element is the chat's last conversation — the provider fallback a
	// dormant chat's activeProviderId derives from.
	conversations map[string][]engineagents.ChatConversation
	convErr       error

	selection    selectionCall
	selectionErr error

	// terminalWait is the standing "this chat's CLI is parked on a modal we
	// cannot answer" verdict, keyed by chat id. A chat absent from the map is not
	// waiting, which is the answer for every chat unless a test says otherwise.
	terminalWait map[string]domain.AgentTerminalWait
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

func (configurableListGetUsecase) IngestHookDelivery(
	_ context.Context,
	_, _, _, _, _ string,
	_ []byte,
) error {
	return nil
}

func (u *configurableListGetUsecase) ListChatsByWorkspace(
	_ context.Context,
	wsID string,
) ([]domain.Chat, error) {
	u.listWsIDs = append(u.listWsIDs, wsID)
	if u.listErr != nil {
		return nil, u.listErr
	}
	return u.chats, nil
}

func (u *configurableListGetUsecase) GetChat(
	_ context.Context,
	_ string,
) (domain.Chat, error) {
	if u.getErr != nil {
		return domain.Chat{}, u.getErr
	}
	return u.chat, nil
}

func (u *configurableListGetUsecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	return u.terminalWait[chatID]
}

func (*configurableListGetUsecase) ReadMessages(
	context.Context, string, int, int, int,
) (domain.LedgerPage, error) {
	return domain.LedgerPage{}, nil
}

func (*configurableListGetUsecase) SubmitPrompt(
	context.Context, string, string, string,
) (domain.AgentPromptSubmission, error) {
	return domain.AgentPromptSubmission{}, nil
}

func (*configurableListGetUsecase) SlashCatalog(
	context.Context, string,
) (engineagents.SlashCatalog, error) {
	return engineagents.SlashCatalog{}, nil
}

func (u *configurableListGetUsecase) LiveRunnerForChat(
	_ context.Context,
	chatID string,
) (engineagents.Runner, error) {
	if u.liveErr != nil {
		return engineagents.Runner{}, u.liveErr
	}
	runner, ok := u.liveRunners[chatID]
	if !ok {
		return engineagents.Runner{}, agentrunner.ErrNotFound
	}
	return runner, nil
}

func (u *configurableListGetUsecase) ConversationsForChat(
	_ context.Context,
	chatID string,
) ([]engineagents.ChatConversation, error) {
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

func (configurableListGetUsecase) Compact(
	_ context.Context,
	_ string,
) error {
	return nil
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

func (c *configurableListGetUsecase) SetChatSelection(
	_ context.Context,
	chatID, model, effort string,
) error {
	c.selection = selectionCall{chatID: chatID, model: model, effort: effort}
	return c.selectionErr
}

func (configurableListGetUsecase) DispatchMCP(
	_ context.Context,
	_, _ string,
	_ []byte,
) ([]byte, bool, error) {
	return nil, false, nil
}

func (configurableListGetUsecase) PurgeChat(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (configurableListGetUsecase) ResolveProviders(
	_ context.Context,
) ([]domain.AgentProvider, error) {
	return nil, nil
}

func (configurableListGetUsecase) ReplaceProviderPreferences(
	_ context.Context,
	_ []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	return nil, nil
}

// TestList_Success proves List reads the :wsId path param (Task 3: nested
// under .../workspaces/:wsId/chats), forwards it to
// ListChatsByWorkspace, and returns every chat as wire DTOs.
func TestList_Success(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.Chat{
			{ID: "c1", WorkspaceID: "ws1", CreatedAt: time.Unix(1, 0).UTC()},
			{ID: "c2", WorkspaceID: "ws1", CreatedAt: time.Unix(2, 0).UTC()},
		},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
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
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
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
		chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws1"}},
		// No live runner for c1: the chat is dormant.
		conversations: map[string][]engineagents.ChatConversation{
			"c1": {
				{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1", FirstSeenAt: time.Unix(1, 0).UTC()},
				{ChatID: "c1", ProviderID: "vendor-b", SessionID: "sess-2", FirstSeenAt: time.Unix(2, 0).UTC()},
			},
		},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
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
		chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws1"}},
		liveRunners: map[string]engineagents.Runner{
			"c1": {
				ID:              "run-1",
				WorkspaceID:     "ws1",
				ProviderID:      "vendor-b",
				TerminalSession: "term-1",
				CurrentChatID:   "c1",
			},
		},
		conversations: map[string][]engineagents.ChatConversation{
			"c1": {{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1"}},
		},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
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
		chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws1"}},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
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
		chats:   []domain.Chat{{ID: "c1", WorkspaceID: "ws1"}},
		liveErr: errors.New("projection down"),
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
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
		chats:   []domain.Chat{{ID: "c1", WorkspaceID: "ws1"}},
		convErr: errors.New("projection down"),
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
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
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws1", Title: "a title"},
		liveRunners: map[string]engineagents.Runner{
			"c1": {ID: "run-1", ProviderID: "vendor-a", TerminalSession: "term-1", CurrentChatID: "c1"},
		},
		conversations: map[string][]engineagents.ChatConversation{
			"c1": {{ChatID: "c1", ProviderID: "vendor-a", SessionID: "sess-1", FirstSeenAt: time.Unix(1, 0).UTC()}},
		},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1", nil)
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

// TestList_CarriesTerminalWaitForAWaitingChatAndOmitsItForOthers proves the list
// route joins in TerminalWait per chat, the same way it joins in the live runner
// and the conversation history: a chat the detector has flagged carries the
// verdict on the wire, and a chat it has not carries no field at all — the field
// must be genuinely ABSENT for the untouched chat, not present-and-null, since
// that absence is the whole point of the *bool for "was this ever computed".
func TestList_CarriesTerminalWaitForAWaitingChatAndOmitsItForOthers(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.Chat{
			{ID: "c1", WorkspaceID: "ws1"},
			{ID: "c2", WorkspaceID: "ws1"},
		},
		terminalWait: map[string]domain.AgentTerminalWait{
			"c1": {Waiting: true, Kind: domain.AgentTerminalWaitTrust},
		},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}}

	h.List(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data []struct {
			ID           string                    `json:"id"`
			TerminalWait *dto.AgentTerminalWaitDTO `json:"terminalWait"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 2)

	require.NotNil(t, envelope.Data[0].TerminalWait, "c1 is waiting and must carry the verdict")
	assert.Equal(t, domain.AgentTerminalWaitTrust, envelope.Data[0].TerminalWait.Kind)
	assert.Nil(t, envelope.Data[1].TerminalWait, "c2 is not waiting and the field must be absent")

	assert.Equal(t, 1, strings.Count(rec.Body.String(), `"terminalWait"`),
		"the key itself must appear exactly once — present for c1, genuinely omitted (not null) for c2")
}

// TestGet_CarriesTerminalWait proves the single-chat GET response joins in the
// same TerminalWait verdict the list route does, so a client opening one chat
// directly (a deep link, a reload) sees the banner without waiting on the
// lifecycle socket to repeat it.
func TestGet_CarriesTerminalWait(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws1"},
		terminalWait: map[string]domain.AgentTerminalWait{
			"c1": {Waiting: true, Kind: domain.AgentTerminalWaitTrust},
		},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"terminalWait":{"kind":"workspace_trust"}`)
}

// TestGet_ConversationsIsNeverNull proves the detail endpoint carries `conversations`
// (the successor of the deleted `segments`) as [] rather than null on a chat that has
// hosted none — the FE maps over it unguarded.
func TestGet_ConversationsIsNeverNull(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws1"},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1", nil)
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
		chat:    domain.Chat{ID: "c1", WorkspaceID: "ws1"},
		liveErr: errors.New("projection down"),
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1", nil)
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
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws-other"},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c1", nil)
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
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/chats/missing", nil)
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
	h := newChatHandlers(uc)

	body := []byte(`{"title":"My Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/c-1/rename?source=agent", body)
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
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c-1", WorkspaceID: "ws-other"}}
	h := newChatHandlers(uc)

	body := []byte(`{"title":"My Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c-1/rename", body)
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
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/c-1/rename", []byte("{not json"))
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
	h := newChatHandlers(uc)

	body := []byte(`{"title":"My Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/missing/rename", body)
	ctx.Params = gin.Params{{Key: "id", Value: "missing"}}

	h.Rename(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestDelete_Success proves Delete forwards the path id to the tree usecase's
// cascading delete and responds 202 with an empty body on success (Task 5).
//
// It goes through the TREE and not straight to PurgeChat because a chat's
// children are threads of it and go with it — see Handlers.Delete.
func TestDelete_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	tree := &fakeChatTree{}
	var frames []folderFrame
	h := newFolderHandlersWith(uc, tree, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	assert.Equal(t, "c-1", tree.gotPurge)
	assert.Empty(t, uc.purgeCalls, "the handler must not purge behind the tree's back")
}

// A chat delete takes any folder inside its subtree with it, and those folders
// are a plain row with no projection to announce them — so the handler does.
func TestDelete_AnnouncesTheFoldersTheCascadeTook(t *testing.T) {
	tree := &fakeChatTree{deletion: agentusecase.ChatDeletion{
		Chats:   []string{"c-2", "c-1"},
		Folders: []string{"f-1"},
		Shifted: []domain.ChatFolder{{ID: "f-0", WorkspaceID: "ws-1"}},
	}}
	var frames []folderFrame
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c-1", WorkspaceID: "ws-1"}}
	h := newFolderHandlersWith(uc, tree, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Len(t, frames, 2)
	assert.Equal(t, folderFrame{folderID: "f-1", workspaceID: "ws-1", kind: "folder_deleted"}, frames[0])
	assert.Equal(t, folderFrame{folderID: "f-0", workspaceID: "ws-1", kind: "folder_updated"}, frames[1])
}

// TestDelete_WrongWorkspace404s proves the by-id scope check
// (requireChatInWorkspace): a chat anchored to a DIFFERENT workspace than the
// :wsId path param 404s before PurgeChat is ever called (Task 5).
func TestDelete_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c-1", WorkspaceID: "ws-other"}}
	tree := &fakeChatTree{}
	var frames []folderFrame
	h := newFolderHandlersWith(uc, tree, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.purgeCalls, "PurgeChat must never be called once the scope check 404s")
	assert.Empty(t, tree.gotPurge, "and neither must the cascade behind it")
}

// TestDelete_UsecaseError proves a cascade failure surfaces as a mapped error
// response rather than a 202.
func TestDelete_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	var frames []folderFrame
	h := newFolderHandlersWith(uc, &fakeChatTree{err: agentchat.ErrNotFound}, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/missing", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "missing"}}

	h.Delete(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func (configurableListGetUsecase) ReadActivity(
	context.Context, string, int64, int,
) (agentusecase.ChatActivity, error) {
	return agentusecase.ChatActivity{}, nil
}

func (configurableListGetUsecase) ReadToolPayload(
	context.Context, string, string, string,
) ([]byte, error) {
	return nil, nil
}

func (configurableListGetUsecase) ReadPendingChoices(
	context.Context, string,
) ([]domain.ActivityChoice, error) {
	return nil, nil
}

func (configurableListGetUsecase) AnswerableChoiceIDs(
	string, []domain.ActivityChoice,
) []string {
	return nil
}

func (configurableListGetUsecase) AnswerChoice(
	context.Context, string, string, []string, string, []byte,
) error {
	return nil
}

func (configurableListGetUsecase) PendingAnswer(string) (agentusecase.PendingAnswer, bool) {
	return agentusecase.PendingAnswer{}, false
}

func (configurableListGetUsecase) AwaitAnswer(
	context.Context, string,
) (agentusecase.HookAnswer, error) {
	return agentusecase.HookAnswer{}, nil
}

func (configurableListGetUsecase) AbandonAnswer(context.Context, string) error { return nil }

func (configurableListGetUsecase) Telemetry(string) (engineagents.Telemetry, bool) {
	return engineagents.Telemetry{}, false
}

// TestSetSelection_ForwardsTheWholeSelection proves the endpoint decodes both
// halves and forwards them together with the path id, answering 202 with an empty
// body.
func TestSetSelection_ForwardsTheWholeSelection(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	body := []byte(`{"model":"opus","effort":"high"}`)
	ctx, rec := newTestContext(t, http.MethodPatch, "/v0/chats/c-1/selection", body)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.SetSelection(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	require.Len(t, uc.selectionCalls, 1)
	assert.Equal(t, selectionCall{chatID: "c-1", model: "opus", effort: "high"}, uc.selectionCalls[0])
}

// TestSetSelection_EmptyBodyClearsBothHalves: the body is the WHOLE selection, so
// an omitted field is a cleared one — that is how a user returns a chat to the
// provider's own default.
func TestSetSelection_EmptyBodyClearsBothHalves(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPatch, "/v0/chats/c-1/selection", []byte(`{}`))
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.SetSelection(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Len(t, uc.selectionCalls, 1)
	assert.Equal(t, selectionCall{chatID: "c-1"}, uc.selectionCalls[0])
}

func TestSetSelection_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c-1", WorkspaceID: "ws-other"}}
	h := newChatHandlers(uc)

	body := []byte(`{"model":"opus"}`)
	ctx, rec := newTestContext(t, http.MethodPatch,
		"/v0/projects/p1/repos/r1/workspaces/ws1/chats/c-1/selection", body)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c-1"}}

	h.SetSelection(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.selectionCalls, "the scope check must 404 before the usecase is reached")
}

func TestSetSelection_BadJSON(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPatch, "/v0/chats/c-1/selection", []byte("{not json"))
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.SetSelection(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.selectionCalls)
}

// TestSetSelection_RejectedValueSurfacesAs400: a value outside the provider's
// declared catalogue is a client error, and the mapped apperr must reach the
// client as one rather than as an opaque 500.
func TestSetSelection_RejectedValueSurfacesAs400(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{selectionErr: fmt.Errorf("no such model: %w", apperr.ErrInvalidArgument)}
	h := newChatHandlers(uc)

	body := []byte(`{"model":"telepathy"}`)
	ctx, rec := newTestContext(t, http.MethodPatch, "/v0/chats/c-1/selection", body)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.SetSelection(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
