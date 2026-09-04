package chat_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// dispatchRecord captures what the MCP route actually handed the usecase, so a
// test can assert on the URL→handler binding rather than on a param map it set
// itself.
type dispatchRecord struct {
	runnerID string
	token    string
	message  []byte
}

// stubChatTree answers every Chats-panel tree call with an empty result. These
// tests are about the routing table — that a URL reaches a handler at all — so
// the tree behind it only has to exist.
type stubChatTree struct{}

func (stubChatTree) CreateChat(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ agentusecase.WorktreeSpec,
) (string, string, error) {
	return "c1", "run-1", nil
}

func (stubChatTree) ListInRepo(
	_ context.Context,
	_ string,
) ([]domain.Chat, error) {
	return nil, nil
}

func (stubChatTree) Create(
	_ context.Context,
	_ agentusecase.CreateInput,
) (domain.Chat, []domain.Chat, error) {
	return domain.Chat{}, nil, nil
}

func (stubChatTree) Rename(
	_ context.Context,
	_ string,
	_ string,
) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubChatTree) Move(
	_ context.Context,
	_ string,
	_ agentusecase.MoveInput,
) (domain.Chat, []domain.Chat, error) {
	return domain.Chat{}, nil, nil
}

func (stubChatTree) Delete(
	_ context.Context,
	_ string,
) ([]domain.Chat, error) {
	return nil, nil
}

func (stubChatTree) PlaceChat(
	_ context.Context,
	_ string,
	_ string,
	_ agentusecase.PlaceInput,
) (domain.Chat, []domain.Chat, error) {
	return domain.Chat{}, nil, nil
}

func (stubChatTree) DeleteChat(
	_ context.Context,
	_ string,
) (agentusecase.ChatDeletion, error) {
	return agentusecase.ChatDeletion{}, nil
}

func (stubChatTree) DeletePreview(
	_ context.Context,
	_ string,
) (int, int, error) {
	return 0, 0, nil
}

// stubUsecase is a VALUE receiver stub throughout, so recording goes through a
// pointer field rather than through the receiver.
type stubUsecase struct {
	dispatch *dispatchRecord
}

func (stubUsecase) SpawnChat(
	_ context.Context,
	_ string,
	_ string,
) (string, string, error) {
	return "chat-1", "seg-1", nil
}

func (stubUsecase) IngestHook(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ []byte,
) error {
	return nil
}

func (stubUsecase) IngestHookDelivery(
	_ context.Context,
	_, _, _, _, _ string,
	_ []byte,
) error {
	return nil
}

func (stubUsecase) ListChatsByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.Chat, error) {
	return nil, nil
}

func (stubUsecase) ListChatsInRepo(
	_ context.Context,
	_ string,
) ([]domain.Chat, error) {
	return nil, nil
}

// GetChat answers a workspace-less chat: TestRegisterMountsRoutes dials the
// repo-scoped mount, which carries no :wsId for requireChatInWorkspace's scope
// check to compare against, so this only has to satisfy the existence half.
func (stubUsecase) GetChat(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	return domain.Chat{ID: id}, nil
}

func (stubUsecase) ReadMessages(
	context.Context, string, int, int, int,
) (domain.LedgerPage, error) {
	return domain.LedgerPage{Items: []domain.LedgerMessage{}}, nil
}

func (stubUsecase) SubmitPrompt(
	context.Context, string, string, string,
) (domain.AgentPromptSubmission, error) {
	return domain.AgentPromptSubmission{RunnerID: "run-2", TerminalSessionID: "term-2"}, nil
}

func (stubUsecase) SlashCatalog(
	context.Context, string,
) (engineagents.SlashCatalog, error) {
	return engineagents.SlashCatalog{Items: []engineagents.SlashCatalogItem{}}, nil
}

// LiveRunnerForChat answers agentrunner.ErrNotFound — "this chat is DORMANT", the
// honest answer for a stub that starts no process, and not a failure: a live-runner row
// exists exactly while a PTY does, so its absence IS the liveness verdict. The read
// routes therefore still serve 200 with empty liveRunnerId/terminalSessionId, which is
// what the mount test asserts on.
func (stubUsecase) LiveRunnerForChat(
	_ context.Context,
	_ string,
) (engineagents.Runner, error) {
	return engineagents.Runner{}, agentrunner.ErrNotFound
}

func (stubUsecase) ConversationsForChat(
	_ context.Context,
	_ string,
) ([]engineagents.ChatConversation, error) {
	return nil, nil
}

func (stubUsecase) SwitchProvider(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "seg-2", nil
}

func (stubUsecase) ResumeChat(
	_ context.Context,
	_ string,
) (string, error) {
	return "seg-2", nil
}

func (stubUsecase) Compact(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (stubUsecase) StopChat(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (stubUsecase) SwitchToTerminal(
	_ context.Context,
	_ string,
) (string, error) {
	return "", nil
}

func (stubUsecase) SwitchToNative(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (stubUsecase) AttachedTerminalSession(_ string) (string, bool) {
	return "", false
}

func (stubUsecase) HasLiveAPIConnection(_ string) bool {
	return false
}

func (stubUsecase) AssembleHandoff(
	_ context.Context,
	_ string,
) (string, error) {
	return "", nil
}

func (stubUsecase) RenameChat(
	_ context.Context,
	_, _, _ string,
) error {
	return nil
}

func (stubUsecase) SetChatSelection(
	_ context.Context,
	_, _, _ string,
) error {
	return nil
}

func (s stubUsecase) DispatchMCP(
	_ context.Context,
	runnerID, token string,
	message []byte,
) ([]byte, bool, error) {
	if s.dispatch != nil {
		*s.dispatch = dispatchRecord{runnerID: runnerID, token: token, message: message}
	}
	return []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`), true, nil
}

func (stubUsecase) PurgeChat(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (stubUsecase) Promote(
	_ context.Context,
	chatID string,
) (domain.Chat, error) {
	return domain.Chat{ID: chatID, WorkspaceID: "ws-promoted"}, nil
}

func (stubUsecase) ResolveProviders(
	_ context.Context,
) ([]domain.AgentProvider, error) {
	return nil, nil
}

func (stubUsecase) ReplaceProviderPreferences(
	_ context.Context,
	_ []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	return nil, nil
}

func (stubUsecase) DefaultPermissionLevel(
	_ context.Context,
) (string, error) {
	return agentusecase.PermissionFullAuto, nil
}

func (stubUsecase) SetDefaultPermissionLevel(
	_ context.Context,
	_ string,
) error {
	return nil
}

// TestRegisterMountsRoutes proves Register mounts every agent route nested
// under the repo-scoped group (Task 17: .../repos/:repoId/chats/...), including
// the WS upgrade route delegating to the supplied handler.
func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	settingsRG := r.Group("/v0")
	wsHit := false
	uc := stubUsecase{}
	chat.Register(repoScoped, settingsRG, uc, uc, uc, uc, uc, stubChatTree{}, nil, nil,
		func(c *gin.Context) {
			wsHit = true
			c.Status(http.StatusOK)
		})

	const base = "/v0/projects/p1/repos/r1"
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, base + "/chats"},
		{http.MethodGet, base + "/chats"},
		{http.MethodGet, base + "/chats/c1"},
		{http.MethodGet, base + "/chats/c1/messages"},
		{http.MethodPost, base + "/chats/c1/prompts"},
		{http.MethodGet, base + "/chats/c1/slash-catalog"},
		{http.MethodPost, base + "/chats/c1/switch"},
		{http.MethodPost, base + "/chats/c1/rename"},
		{http.MethodGet, base + "/chats/c1/handoff"},
		{http.MethodPatch, base + "/chats/c1/placement"},
		{http.MethodDelete, base + "/chats/c1"},
		{http.MethodGet, base + "/chats/c1/delete-preview"},
		{http.MethodPut, base + "/chats/c1/permission-level"},
		{http.MethodGet, base + "/chats/folders"},
		{http.MethodPost, base + "/chats/folders"},
		{http.MethodPatch, base + "/chats/folders/f1"},
		{http.MethodDelete, base + "/chats/folders/f1"},
		{http.MethodPost, base + "/chats/runners/seg-1/mcp"},
		{http.MethodPost, base + "/chats/hooks"},
		{http.MethodGet, base + "/chats/providers"},
		{http.MethodGet, base + "/chats/ws"},
		{http.MethodPut, "/v0/settings/chat/providers"},
		{http.MethodGet, "/v0/settings/chat/permission-level"},
		{http.MethodPut, "/v0/settings/chat/permission-level"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		if tc.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
	assert.True(t, wsHit, "GET .../chats/ws must delegate to the supplied handler")
}

// TestChatRoutes_OldWorkspaceScopedPathGone proves the pre-Task-17 shape is
// genuinely gone (404, not merely undocumented): Register no longer mounts
// anything under .../workspaces/:wsId at all, since chat.Register only ever
// receives the repo-scoped group now (router.go).
func TestChatRoutes_OldWorkspaceScopedPathGone(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	settingsRG := r.Group("/v0")
	uc := stubUsecase{}
	chat.Register(repoScoped, settingsRG, uc, uc, uc, uc, uc, stubChatTree{}, nil, nil,
		func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet,
		"/v0/projects/p1/repos/r1/workspaces/w1/chats", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRegisterBindsMCPSegIDFromTheURL closes the gap every other MCP test leaves
// open: they set gin's param map by hand, so the route's :segid and the
// handler's ctx.Param("segid") could disagree and still pass. Only a request
// through the REAL router proves the two names are the same name.
//
// The consequence of a mismatch is an empty runner id, which the token check
// then refuses — availability, not an authorization hole — but it would take
// down every agent's tools with nothing else catching it.
func TestRegisterBindsMCPSegIDFromTheURL(
	t *testing.T,
) {
	got := &dispatchRecord{}
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	settingsRG := r.Group("/v0")
	uc := stubUsecase{dispatch: got}
	chat.Register(repoScoped, settingsRG, uc, uc, uc, uc, uc, stubChatTree{}, nil, nil,
		func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

	const path = "/v0/projects/p1/repos/r1/chats/runners/seg-42/mcp"
	req := httptest.NewRequest(http.MethodPost, path,
		strings.NewReader(`{"token":"TOK","rpc":{"jsonrpc":"2.0","id":1,"method":"ping"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "seg-42", got.runnerID, "the handler must read the URL's :segid")
	assert.Equal(t, "TOK", got.token)
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, string(got.message))
}

func (stubUsecase) ReadActivity(
	context.Context, string, int64, int,
) (agentusecase.ChatActivity, error) {
	return agentusecase.ChatActivity{}, nil
}

func (stubUsecase) ReadToolPayload(context.Context, string, string, string) ([]byte, error) {
	return nil, nil
}

// TerminalWait: this stub is never blocked on anything, which is what every
// route-mounting assertion here needs it to be.
func (stubUsecase) TerminalWait(string) domain.AgentTerminalWait {
	return domain.AgentTerminalWait{}
}

func (stubUsecase) ReadPendingChoices(
	context.Context, string,
) ([]domain.ActivityChoice, error) {
	return nil, nil
}

func (stubUsecase) AnswerableChoiceIDs(string, []domain.ActivityChoice) []string { return nil }

func (stubUsecase) AnswerChoice(
	context.Context, string, string, []string, string, []byte,
) error {
	return nil
}

func (stubUsecase) PendingAnswer(string) (agentusecase.PendingAnswer, bool) {
	return agentusecase.PendingAnswer{}, false
}

func (stubUsecase) AwaitAnswer(context.Context, string) (agentusecase.HookAnswer, error) {
	return agentusecase.HookAnswer{}, nil
}

func (stubUsecase) AbandonAnswer(context.Context, string) error { return nil }

func (stubUsecase) SetChatPermissionLevel(
	context.Context, string, string,
) error {
	return nil
}

func (stubUsecase) Telemetry(string) (engineagents.Telemetry, bool) {
	return engineagents.Telemetry{}, false
}
