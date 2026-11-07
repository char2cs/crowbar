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
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
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
	_ agentusecase.CreateChatInput,
) (agentusecase.CreatedChat, error) {
	return agentusecase.CreatedChat{ChatID: "c1", RunnerID: "run-1"}, nil
}

func (stubChatTree) ListInRepo(context.Context, string) ([]domain.Chat, error) { return nil, nil }

func (stubChatTree) ListInHome(context.Context, string) ([]domain.Chat, error) { return nil, nil }

// FolderScope answers a folder in the repo these routes are mounted under.
func (stubChatTree) FolderScope(_ context.Context, id string) (tree.Folder, error) {
	return tree.Folder{ID: id, RepoID: "r1"}, nil
}

func (stubChatTree) Create(
	_ context.Context,
	_ agentusecase.CreateInput,
) (domain.Chat, []domain.Chat, error) {
	return domain.Chat{}, nil, nil
}

func (stubChatTree) Rename(context.Context, string, string) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubChatTree) Move(
	_ context.Context,
	_ string,
	_ agentusecase.MoveInput,
) (domain.Chat, []domain.Chat, error) {
	return domain.Chat{}, nil, nil
}

func (stubChatTree) Delete(context.Context, string) ([]domain.Chat, error) { return nil, nil }

func (stubChatTree) CancelMove(context.Context, string) error { return nil }

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
	_ domain.DeleteConsent,
) (agentusecase.ChatDeletion, error) {
	return agentusecase.ChatDeletion{}, nil
}

func (stubChatTree) MintOwningChat(context.Context, string) (string, error) { return "", nil }

func (stubChatTree) AttachOwningWorkspace(context.Context, string, domain.Workspace) error {
	return nil
}

func (stubChatTree) DiscardOwningChat(context.Context, string) error { return nil }

// stubUsecase is a VALUE receiver stub throughout, so recording goes through a
// pointer field rather than through the receiver.
type stubUsecase struct {
	dispatch *dispatchRecord
}

func (stubUsecase) Hook(
	context.Context, string, string, string, []byte, engineagents.Channel,
) (agentusecase.HookReply, error) {
	return agentusecase.HookReply{}, nil
}

func (stubUsecase) ListChatsByWorkspace(context.Context, string) ([]domain.Chat, error) {
	return nil, nil
}

func (stubUsecase) ListChatsInRepo(context.Context, string) ([]domain.Chat, error) { return nil, nil }

func (stubUsecase) CwdWorkspaceID(context.Context, string) (string, bool, error) {
	return "", false, nil
}

// GetChat answers a workspace-less chat: TestRegisterMountsRoutes dials the
// repo-scoped mount with no Worktrees port wired, so requireChatInWorkspace's
// repo-scope check has nothing to resolve a ground against and this only has
// to satisfy the existence half.
func (stubUsecase) GetChat(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	return domain.Chat{ID: id}, nil
}

func (stubUsecase) ChatSnapshot(
	_ context.Context,
	id string,
) (agentusecase.ChatSnapshot, error) {
	return agentusecase.ChatSnapshot{Chat: domain.Chat{ID: id}, Phase: agentusecase.ChatPhaseDormant}, nil
}

func (s stubUsecase) ChatSnapshotsOf(
	ctx context.Context,
	ids []string,
) ([]agentusecase.ChatSnapshot, error) {
	out := make([]agentusecase.ChatSnapshot, len(ids))
	for i, id := range ids {
		snap, err := s.ChatSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		out[i] = snap
	}
	return out, nil
}

func (stubUsecase) SubmitPrompt(
	context.Context, string, string, string, string, *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	return domain.AgentPromptSubmission{RunnerID: "run-2", TerminalSessionID: "term-2"}, nil
}

func (stubUsecase) SlashCatalog(
	context.Context, string,
) (engineagents.SlashCatalog, error) {
	return engineagents.SlashCatalog{Items: []engineagents.SlashCatalogItem{}}, nil
}

func (stubUsecase) ConversationsForChat(context.Context, string) ([]engineagents.ChatConversation, error) {
	return nil, nil
}

func (stubUsecase) SwitchProvider(context.Context, string, string, string) (string, error) {
	return "seg-2", nil
}

func (stubUsecase) ResumeChat(context.Context, string) (string, error) { return "seg-2", nil }

func (stubUsecase) Compact(context.Context, string) error { return nil }

func (stubUsecase) Retract(context.Context, string, string) (agentusecase.RetractResult, error) {
	return agentusecase.RetractResult{}, nil
}

func (stubUsecase) StopChat(context.Context, string) error { return nil }

func (stubUsecase) KillChat(context.Context, string) error { return nil }

func (stubUsecase) SwitchToTerminal(context.Context, string) (string, error) { return "", nil }

func (stubUsecase) SwitchToNative(context.Context, string) error { return nil }

func (stubUsecase) AttachedTerminalSession(_ string) (string, bool) {
	return "", false
}

func (stubUsecase) AssembleHandoff(context.Context, string) (string, error) { return "", nil }

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

func (s stubUsecase) VerifyRunnerToken(string, string) bool { return false }

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

func (stubUsecase) PurgeChat(context.Context, string) error { return nil }

func (stubUsecase) Promote(
	_ context.Context,
	chatID string,
) (domain.Chat, error) {
	return domain.Chat{ID: chatID, WorkspaceID: "ws-promoted"}, nil
}

func (stubUsecase) DescriptorReports(
	context.Context,
) ([]descriptorcheck.Report, error) {
	return nil, nil
}

func (stubUsecase) InstallDescriptor(context.Context, []byte) (descriptorcheck.Report, error) {
	return descriptorcheck.Report{}, nil
}

func (stubUsecase) ResolveProviders(context.Context) ([]domain.AgentProvider, error) { return nil, nil }

func (stubUsecase) ReplaceProviderPreferences(
	_ context.Context,
	_ []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	return nil, nil
}

func (stubUsecase) DefaultPermissionLevel(context.Context) (string, error) {
	return agentusecase.PermissionFullAuto, nil
}

func (stubUsecase) SetDefaultPermissionLevel(context.Context, string) error { return nil }

func (stubUsecase) ModelManifestFetchEnabled(context.Context) (bool, error) { return true, nil }

func (stubUsecase) SetModelManifestFetchEnabled(context.Context, bool) error { return nil }

// TestRegisterMountsRoutes proves Register mounts every agent route nested
// under the repo-scoped group (Task 17: .../repos/:repoId/chats/...), including
// the WS upgrade route delegating to the supplied handler.
func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	repoScoped := r.Group("/v0/projects/:projectId/repos/:repoId")
	settingsRG := r.Group("/v0")
	uc := stubUsecase{}
	chat.Register(repoScoped, r.Group("/v0/chats/:chatId"), settingsRG, uc, uc, uc, uc, uc, stubChatTree{}, nil, nil, nil)

	const base = "/v0/projects/p1/repos/r1"
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, base + "/chats"},
		{http.MethodGet, base + "/chats"},
		{http.MethodGet, base + "/chats/c1"},
		{http.MethodPost, base + "/chats/c1/prompts"},
		{http.MethodPost, base + "/chats/c1/prompts/r1/retract"},
		{http.MethodGet, base + "/chats/c1/slash-catalog"},
		{http.MethodPost, base + "/chats/c1/attachments"},
		{http.MethodGet, base + "/chats/c1/attachments/f1.png"},
		{http.MethodPost, base + "/chats/c1/switch"},
		{http.MethodPost, "/v0/chats/c1/kill"},
		{http.MethodPost, base + "/chats/c1/rename"},
		{http.MethodGet, base + "/chats/c1/handoff"},
		{http.MethodPatch, base + "/chats/c1/placement"},
		{http.MethodPost, base + "/chats/c1/move/cancel"},
		{http.MethodDelete, base + "/chats/c1"},
		{http.MethodPut, base + "/chats/c1/permission-level"},
		{http.MethodGet, base + "/chats/folders"},
		{http.MethodPost, base + "/chats/folders"},
		{http.MethodPatch, base + "/chats/folders/f1"},
		{http.MethodDelete, base + "/chats/folders/f1"},
		{http.MethodPost, base + "/chats/runners/seg-1/mcp"},
		{http.MethodPost, base + "/chats/runners/seg-1/hooks/claude/tool_pre"},
		{http.MethodGet, base + "/chats/providers"},
		{http.MethodPut, "/v0/settings/chat/providers"},
		{http.MethodGet, "/v0/settings/chat/permission-level"},
		{http.MethodPut, "/v0/settings/chat/permission-level"},
		{http.MethodGet, "/v0/settings/chat/model-manifest-fetch"},
		{http.MethodPut, "/v0/settings/chat/model-manifest-fetch"},
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
	chat.Register(repoScoped, r.Group("/v0/chats/:chatId"), settingsRG, uc, uc, uc, uc, uc, stubChatTree{}, nil, nil, nil)

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
	chat.Register(repoScoped, r.Group("/v0/chats/:chatId"), settingsRG, uc, uc, uc, uc, uc, stubChatTree{}, nil, nil, nil)

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

// TerminalWait: this stub is never blocked on anything, which is what every
// route-mounting assertion here needs it to be.
func (stubUsecase) TerminalWait(string) domain.AgentTerminalWait {
	return domain.AgentTerminalWait{}
}

func (stubUsecase) AnswerTerminalWait(context.Context, string, string) error { return nil }

func (stubUsecase) AnswerChoice(
	context.Context, string, string, []string, string, []byte,
) error {
	return nil
}

func (stubUsecase) HoldsAnswer(string) bool {
	return false
}

func (stubUsecase) AwaitAnswer(context.Context, string) (agentusecase.HookAnswer, error) {
	return agentusecase.HookAnswer{}, nil
}

func (stubUsecase) SetChatPermissionLevel(
	context.Context, string, string,
) error {
	return nil
}

func (stubUsecase) Telemetry(string) (engineagents.Telemetry, bool) {
	return engineagents.Telemetry{}, false
}

func (stubUsecase) TelemetryOnChatSurface(context.Context, string) bool { return true }

func (stubUsecase) UploadAttachment(
	context.Context, string, agentusecase.UploadAttachmentInput,
) (agentusecase.StoredAttachment, error) {
	return agentusecase.StoredAttachment{}, nil
}

func (stubUsecase) ReadAttachment(context.Context, string, string) ([]byte, string, error) {
	return nil, "", nil
}
