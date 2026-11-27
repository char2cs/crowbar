package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

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

func TestCreate_ForwardsWhatTheRequestNames(t *testing.T) {
	const wsPath, repoPath = "/v0/projects/p1/repos/r1/workspaces/ws-1/chats", "/v0/projects/p1/repos/r1/chats"
	wsParams, repoParams := gin.Params{{Key: "wsId", Value: "ws-1"}}, gin.Params{{Key: "repoId", Value: "r1"}}
	onWS, onWS9 := createChatCall{WorkspaceID: "ws-1", ProviderID: "vendor-a"}, createChatCall{WorkspaceID: "ws-9", ProviderID: "vendor-a"}
	var none createChatCall
	for _, tc := range []struct {
		name, path string
		params     gin.Params
		body       string
		surface    string
		treeErr    error
		code       int
		calls      int
		want       createChatCall
	}{
		{"the surface", wsPath, wsParams, `{"provider":"vendor-a","surface":"terminal"}`, "terminal", nil, http.StatusCreated, 1, onWS},
		{"no surface is the provider's default", wsPath, wsParams, `{"provider":"vendor-a"}`, "", nil, http.StatusCreated, 1, onWS},
		{"an unknown surface is refused", wsPath, wsParams, `{"provider":"vendor-a","surface":"holodeck"}`, "", nil, http.StatusBadRequest, 0, none},
		{"the parent", wsPath, wsParams, `{"provider":"vendor-a","parentId":"parent-chat"}`, "", nil, http.StatusCreated, 1, createChatCall{WorkspaceID: "ws-1", ProviderID: "vendor-a", ParentID: "parent-chat"}},
		{"bad JSON", wsPath, wsParams, "{not json", "", nil, http.StatusBadRequest, 0, none},
		{"a tree failure", wsPath, wsParams, `{"provider":"vendor-a"}`, "", errors.New("boom"), http.StatusInternalServerError, 1, onWS},
		{"the body's workspace at the repo mount", repoPath, repoParams, `{"provider":"vendor-a","workspaceId":"ws-9"}`, "", nil, http.StatusCreated, 1, onWS9},
		{"no workspace at all is workspace-less", repoPath, repoParams, `{"provider":"vendor-a"}`, "", nil, http.StatusCreated, 1, createChatCall{ProviderID: "vendor-a"}},
		{"ownWorktree at the repo mount", repoPath, repoParams, `{"provider":"vendor-a","parentId":"parent-chat","ownWorktree":true}`, "", nil, http.StatusCreated, 1, createChatCall{ProviderID: "vendor-a", ParentID: "parent-chat", OwnWorktree: true}},
		{"ownWorktree is ignored when the path names a workspace", wsPath, wsParams, `{"provider":"vendor-a","ownWorktree":true}`, "", nil, http.StatusCreated, 1, onWS},
		{"ownWorktree is ignored when the body names a workspace", repoPath, repoParams, `{"provider":"vendor-a","workspaceId":"ws-9","ownWorktree":true}`, "", nil, http.StatusCreated, 1, onWS9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}, err: tc.treeErr}
			h := newChatHandlersWith(&fakeAgentUsecase{}, tree)
			ctx, rec := newTestContext(t, http.MethodPost, tc.path, []byte(tc.body))
			ctx.Params = tc.params

			h.Create(ctx)

			assert.Equal(t, tc.code, rec.Code)
			assert.Equal(t, tc.calls, tree.createChatCalls)
			assert.Equal(t, tc.want, tree.gotCreate2)
			assert.Equal(t, tc.surface, tree.gotSurface)
		})
	}
}

func TestRegression_Create_ForwardsTheTypedBranchNameOnAnOwnWorktreeCreate(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	h := newChatHandlersWith(&fakeAgentUsecase{}, tree)

	body := []byte(`{"provider":"vendor-a","ownWorktree":true,"branch":"feature/typed-name","parentId":"c-owner"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/chats", body)
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}}

	h.Create(ctx)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, agentusecase.WorktreeFork, tree.gotWorktree.Mode)
	assert.Equal(t, "feature/typed-name", tree.gotWorktree.Branch,
		"the typed branch name must reach WorktreeSpec, not be dropped in favour of the auto-generated one")
}

type configurableListGetUsecase struct {
	chats         []domain.Chat
	listErr       error
	listWsIDs     []string
	listInRepoIDs []string

	chat   domain.Chat
	getErr error

	cwdWorkspaceID string
	cwdErr         error

	renamed []string

	liveRunners map[string]engineagents.Runner
	liveErr     error

	conversations map[string][]engineagents.ChatConversation
	convErr       error

	selection    selectionCall
	selectionErr error

	terminalWait map[string]domain.AgentTerminalWait
}

func (*configurableListGetUsecase) TelemetryOnChatSurface(context.Context, string) bool {
	return true
}

func (u *configurableListGetUsecase) ChatSnapshot(
	_ context.Context,
	chatID string,
) (agentusecase.ChatSnapshot, error) {
	if u.liveErr != nil {
		return agentusecase.ChatSnapshot{}, u.liveErr
	}
	chat := u.chat
	for _, c := range u.chats {
		if c.ID == chatID {
			chat = c
		}
	}
	snap := agentusecase.ChatSnapshot{
		Chat: chat, Version: 1, Phase: agentusecase.ChatPhaseDormant,
		TerminalWait: u.terminalWait[chatID],
	}
	if r, ok := u.liveRunners[chatID]; ok {
		snap.Live, snap.Phase = &r, agentusecase.ChatPhaseLive
		snap.AttachedSessionID, _ = u.AttachedTerminalSession(r.ID)
	}
	return snap, nil
}

func (u *configurableListGetUsecase) ChatSnapshotsOf(
	ctx context.Context,
	ids []string,
) ([]agentusecase.ChatSnapshot, error) {
	out := make([]agentusecase.ChatSnapshot, len(ids))
	for i, id := range ids {
		snap, err := u.ChatSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		out[i] = snap
	}
	return out, nil
}

func (configurableListGetUsecase) Hook(
	context.Context, string, string, string, []byte, engineagents.Channel,
) (agentusecase.HookReply, error) {
	return agentusecase.HookReply{}, nil
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

func (u *configurableListGetUsecase) ListChatsInRepo(
	_ context.Context,
	repoID string,
) ([]domain.Chat, error) {
	u.listInRepoIDs = append(u.listInRepoIDs, repoID)
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

func (u *configurableListGetUsecase) CwdWorkspaceID(
	_ context.Context,
	_ string,
) (string, bool, error) {
	if u.cwdErr != nil {
		return "", false, u.cwdErr
	}
	return u.cwdWorkspaceID, u.cwdWorkspaceID != "", nil
}

func (u *configurableListGetUsecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	return u.terminalWait[chatID]
}

func (*configurableListGetUsecase) SubmitPrompt(
	context.Context, string, string, string, string, *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	return domain.AgentPromptSubmission{}, nil
}

func (*configurableListGetUsecase) SlashCatalog(
	context.Context, string,
) (engineagents.SlashCatalog, error) {
	return engineagents.SlashCatalog{}, nil
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

func (configurableListGetUsecase) SwitchProvider(context.Context, string, string, string) (string, error) {
	return "", nil
}

func (configurableListGetUsecase) ResumeChat(context.Context, string) (string, error) { return "", nil }

func (configurableListGetUsecase) StopShell(context.Context, string, string) error { return nil }

func (configurableListGetUsecase) Compact(context.Context, string) error { return nil }

func (configurableListGetUsecase) StopChat(context.Context, string) error { return nil }

func (configurableListGetUsecase) KillChat(context.Context, string) error { return nil }

func (configurableListGetUsecase) SwitchToTerminal(context.Context, string) (string, error) {
	return "", nil
}

func (configurableListGetUsecase) SwitchToNative(context.Context, string) error { return nil }

func (configurableListGetUsecase) AttachedTerminalSession(_ string) (string, bool) {
	return "", false
}

func (configurableListGetUsecase) AssembleHandoff(context.Context, string) (string, error) {
	return "", nil
}

func (u *configurableListGetUsecase) RenameChat(
	_ context.Context,
	chatID, _, _ string,
) error {
	u.renamed = append(u.renamed, chatID)
	return nil
}

func (c *configurableListGetUsecase) SetChatSelection(
	_ context.Context,
	chatID, model, effort string,
) error {
	c.selection = selectionCall{chatID: chatID, model: model, effort: effort}
	return c.selectionErr
}

func (configurableListGetUsecase) VerifyRunnerToken(string, string) bool { return false }

func (configurableListGetUsecase) DispatchMCP(
	_ context.Context,
	_, _ string,
	_ []byte,
) ([]byte, bool, error) {
	return nil, false, nil
}

func (configurableListGetUsecase) DispatchRelayMCP(
	_ context.Context,
	_, _ string,
	_ []byte,
) ([]byte, bool, error) {
	return nil, false, nil
}

func (configurableListGetUsecase) PurgeChat(context.Context, string) error { return nil }

func (configurableListGetUsecase) DescriptorReports(
	context.Context,
) ([]descriptorcheck.Report, error) {
	return nil, nil
}

func (configurableListGetUsecase) InstallDescriptor(context.Context, []byte) (descriptorcheck.Report, error) {
	return descriptorcheck.Report{}, nil
}

func (configurableListGetUsecase) ResolveProviders(context.Context) ([]domain.AgentProvider, error) {
	return nil, nil
}

func (configurableListGetUsecase) ReplaceProviderPreferences(
	_ context.Context,
	_ []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	return nil, nil
}

func (configurableListGetUsecase) DefaultPermissionLevel(context.Context) (string, error) {
	return agentusecase.PermissionFullAuto, nil
}

func (configurableListGetUsecase) SetDefaultPermissionLevel(context.Context, string) error {
	return nil
}

func (configurableListGetUsecase) ModelManifestFetchEnabled(context.Context) (bool, error) {
	return true, nil
}

func (configurableListGetUsecase) SetModelManifestFetchEnabled(context.Context, bool) error {
	return nil
}

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

func TestList_NoPathWorkspace_ScopesToTheRepo(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chats: []domain.Chat{
			{ID: "c1", WorkspaceID: "ws1"},
			{ID: "c2", WorkspaceID: "ws2"},
		},
	}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats", nil)
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}}

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
	assert.Equal(t, "c2", envelope.Data[1].ID)

	assert.Equal(t, []string{"r1"}, uc.listInRepoIDs,
		"the repo in the URL is what the list must be scoped to")
	assert.Empty(t, uc.listWsIDs, "ListChatsByWorkspace must not run when no workspace is named")
}

type chatRow struct {
	ID                string `json:"id"`
	LiveRunnerID      string `json:"liveRunnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
	ActiveProviderID  string `json:"activeProviderId"`
}

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

func TestGet_NoPathWorkspace_AnyWorkspaceIsVisible(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws-other"},
	}
	h := newWorktreeHandlers(uc, &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-other", ProjectID: "p1", RepoID: "r1"},
	}})

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRegression_RepoMountRefusesAChatThatRunsInTheProjectHome(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws-home"},
	}
	h := newWorktreeHandlers(uc, &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-home", ProjectID: "p1", Kind: domain.WorkspaceKindHome},
	}})

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRegression_RepoMountRefusesAChatThatRunsInAnotherRepo(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws-r2"},
	}
	h := newWorktreeHandlers(uc, &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-r2", ProjectID: "p1", RepoID: "r2"},
	}})

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c1"}}

	h.Get(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRegression_RepoMountRefusesAHomeChatOnAMutatingRoute(
	t *testing.T,
) {
	uc := &configurableListGetUsecase{
		chat: domain.Chat{ID: "c1", WorkspaceID: "ws-home"},
	}
	h := newWorktreeHandlers(uc, &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-home", ProjectID: "p1", Kind: domain.WorkspaceKindHome},
	}})

	body := []byte(`{"title":"renamed through the wrong mount"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/chats/c1/rename", body)
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c1"}}

	h.Rename(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.renamed, "RenameChat must never be called once the scope check 404s")
}

func TestGet_RepoMount_BubbleIsHeldToTheRepoItsWalkLandsIn(
	t *testing.T,
) {
	rows := []domain.Workspace{
		{ID: "ws-r1", ProjectID: "p1", RepoID: "r1"},
		{ID: "ws-r2", ProjectID: "p1", RepoID: "r2"},
	}

	for _, tc := range []struct {
		name string
		cwd  string
		want int
	}{
		{name: "lands in this repo", cwd: "ws-r1", want: http.StatusOK},
		{name: "lands in another repo", cwd: "ws-r2", want: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc := &configurableListGetUsecase{
				chat:           domain.Chat{ID: "c1"},
				cwdWorkspaceID: tc.cwd,
			}
			h := newWorktreeHandlers(uc, &fakeWorktreeReads{rows: rows})

			ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats/c1", nil)
			ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c1"}}

			h.Get(ctx)

			assert.Equal(t, tc.want, rec.Code)
		})
	}
}

func TestGet_RepoMount_ARowWithNoResolvableGroundIsStillServed(
	t *testing.T,
) {
	for _, tc := range []struct {
		name string
		uc   *configurableListGetUsecase
	}{
		{
			name: "the walk resolves nothing",
			uc:   &configurableListGetUsecase{chat: domain.Chat{ID: "c1"}},
		},
		{
			name: "the walk itself failed",
			uc: &configurableListGetUsecase{
				chat:   domain.Chat{ID: "c1"},
				cwdErr: errors.New("projection unreadable"),
			},
		},
		{
			name: "the workspace row is unreadable",
			uc:   &configurableListGetUsecase{chat: domain.Chat{ID: "c1", WorkspaceID: "ws-gone"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newWorktreeHandlers(tc.uc, &fakeWorktreeReads{})

			ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats/c1", nil)
			ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c1"}}

			h.Get(ctx)

			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestRename_PostsTitleAndSource(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	body := []byte(`{"title":"My Title"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/chats/c-1/rename?source=agent", body)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Rename(ctx)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes())

	require.Len(t, uc.renameCalls, 1)
	assert.Equal(t, "c-1", uc.renameCalls[0].chatID)
	assert.Equal(t, "My Title", uc.renameCalls[0].title)
	assert.Equal(t, "agent", uc.renameCalls[0].source)
}

func (configurableListGetUsecase) AnswerTerminalWait(context.Context, string, string) error {
	return nil
}

func (configurableListGetUsecase) AnswerChoice(
	context.Context, string, string, []string, string, []byte,
) error {
	return nil
}

func (configurableListGetUsecase) HoldsAnswer(string) bool {
	return false
}

func (configurableListGetUsecase) AwaitAnswer(
	context.Context, string,
) (agentusecase.HookAnswer, error) {
	return agentusecase.HookAnswer{}, nil
}

func (configurableListGetUsecase) SetChatPermissionLevel(
	context.Context, string, string,
) error {
	return nil
}

func (configurableListGetUsecase) Telemetry(string) (engineagents.Telemetry, bool) {
	return engineagents.Telemetry{}, false
}

func (configurableListGetUsecase) UploadAttachment(
	context.Context, string, agentusecase.UploadAttachmentInput,
) (agentusecase.StoredAttachment, error) {
	return agentusecase.StoredAttachment{}, nil
}

func (configurableListGetUsecase) ReadAttachment(
	context.Context, string, string,
) ([]byte, string, error) {
	return nil, "", nil
}

func TestSetSelection_ForwardsTheWholeSelection(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	body := []byte(`{"model":"opus","effort":"high"}`)
	ctx, rec := newTestContext(t, http.MethodPatch, "/v0/chats/c-1/selection", body)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.SetSelection(ctx)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	require.Len(t, uc.selectionCalls, 1)
	assert.Equal(t, selectionCall{chatID: "c-1", model: "opus", effort: "high"}, uc.selectionCalls[0])
}

func TestSetSelection_EmptyBodyClearsBothHalves(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPatch, "/v0/chats/c-1/selection", []byte(`{}`))
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.SetSelection(ctx)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, uc.selectionCalls, 1)
	assert.Equal(t, selectionCall{chatID: "c-1"}, uc.selectionCalls[0])
}
