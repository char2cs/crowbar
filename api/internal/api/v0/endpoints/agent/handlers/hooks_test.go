package handlers_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

func newTestContext(
	t *testing.T,
	method, path string,
	body []byte,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	return ctx, rec
}

// TestHooks_DecodesAndDispatches proves Hooks decodes the
// {segment_id, provider, event, payload_raw} body and forwards the decoded
// values verbatim to IngestHook, writing a bare 202 on success
// (fail-fast/good-path-async, 00 §4).
func TestHooks_DecodesAndDispatches(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	body := []byte(`{"segment_id":"seg-1","provider":"claude","event":"session_start","payload_raw":"{\"sessionId\":\"sess-1\"}"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/hooks", body)

	h.Hooks(ctx)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())

	require.Len(t, uc.ingestCalls, 1)
	assert.Equal(t, "seg-1", uc.ingestCalls[0].segID)
	assert.Equal(t, "claude", uc.ingestCalls[0].provider)
	assert.Equal(t, "session_start", uc.ingestCalls[0].event)
	assert.Contains(t, string(uc.ingestCalls[0].raw), `"sessionId":"sess-1"`)
}

// TestHooks_BadJSON proves a malformed body is rejected 400 without reaching
// the usecase.
func TestHooks_BadJSON(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/hooks", []byte("{not json"))

	h.Hooks(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.ingestCalls)
}

// TestHooks_UsecaseError proves an IngestHook failure surfaces as a mapped
// error response rather than a 202.
func TestHooks_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{ingestErr: errors.New("boom")}
	h := handlers.New(uc)

	body := []byte(`{"segment_id":"seg-1","provider":"claude","event":"turn_stop","payload_raw":"{}"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/hooks", body)

	h.Hooks(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// fakeAgentUsecase is a configurable AgentUsecase double: each method records
// its call so tests can assert forwarding, and each returns a canned
// error/value so tests can exercise the handlers' error branches.
type fakeAgentUsecase struct {
	ingestCalls []ingestCall
	ingestErr   error

	spawnChatID string
	spawnSegID  string
	spawnErr    error
	spawnCalls  []spawnCall

	switchCalls    []switchCall
	switchNewSegID string
	switchErr      error

	resumeCalls []string
	resumeSegID string
	resumeErr   error

	handoffStr string
	handoffErr error

	renameCalls []renameCall
	renameErr   error

	renameByRunnerCalls []renameByRunnerCall
	renameByRunnerErr   error

	purgeCalls []string
	purgeErr   error

	providers              []engineagent.Descriptor
	providersErr           error
	listProvidersWorkspace string

	// getChat/getChatErr configure GetChat, the call every
	// requireChatInWorkspace scope check (Get/Switch/Rename/Handoff) makes
	// first. The zero value (an empty domain.AgentChat, WorkspaceID "") makes
	// the scope check pass by default for callers that leave :wsId unset too.
	getChat    domain.AgentChat
	getChatErr error
}

type ingestCall struct {
	segID    string
	provider string
	event    string
	raw      []byte
}

type spawnCall struct {
	workspaceID string
	provider    string
}

type switchCall struct {
	chatID   string
	provider string
}

type renameCall struct {
	chatID string
	title  string
	source string
}

type renameByRunnerCall struct {
	runnerID string
	title    string
	source   string
}

func (f *fakeAgentUsecase) SpawnChat(
	_ context.Context,
	workspaceID string,
	provider string,
) (string, string, error) {
	f.spawnCalls = append(f.spawnCalls, spawnCall{workspaceID: workspaceID, provider: provider})
	if f.spawnErr != nil {
		return "", "", f.spawnErr
	}
	return f.spawnChatID, f.spawnSegID, nil
}

func (f *fakeAgentUsecase) IngestHook(
	_ context.Context,
	segID string,
	provider string,
	event string,
	raw []byte,
) error {
	f.ingestCalls = append(f.ingestCalls, ingestCall{segID: segID, provider: provider, event: event, raw: raw})
	return f.ingestErr
}

func (f *fakeAgentUsecase) ListChatsByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.AgentChat, error) {
	return nil, nil
}

func (f *fakeAgentUsecase) GetChat(
	_ context.Context,
	_ string,
) (domain.AgentChat, error) {
	if f.getChatErr != nil {
		return domain.AgentChat{}, f.getChatErr
	}
	return f.getChat, nil
}

// LiveRunnerForChat/ConversationsForChat back the derived runner facts on the chat
// DTOs, which only the List/Get read handlers compose (see
// configurableListGetUsecase in chats_test.go, the double those tests dial in). This
// double answers "dormant, no history" — the honest shape of a chat no runner has
// ever been placed on — so the mutation handlers it does serve never trip over them.
func (f *fakeAgentUsecase) LiveRunnerForChat(
	_ context.Context,
	_ string,
) (domain.AgentRunner, error) {
	return domain.AgentRunner{}, agentrunner.ErrNotFound
}

func (f *fakeAgentUsecase) ConversationsForChat(
	_ context.Context,
	_ string,
) ([]domain.ChatConversation, error) {
	return nil, nil
}

func (f *fakeAgentUsecase) SwitchProvider(
	_ context.Context,
	chatID string,
	provider string,
) (string, error) {
	f.switchCalls = append(f.switchCalls, switchCall{chatID: chatID, provider: provider})
	if f.switchErr != nil {
		return "", f.switchErr
	}
	return f.switchNewSegID, nil
}

func (f *fakeAgentUsecase) ResumeChat(
	_ context.Context,
	chatID string,
) (string, error) {
	f.resumeCalls = append(f.resumeCalls, chatID)
	if f.resumeErr != nil {
		return "", f.resumeErr
	}
	return f.resumeSegID, nil
}

func (f *fakeAgentUsecase) AssembleHandoff(
	_ context.Context,
	_ string,
) (string, error) {
	if f.handoffErr != nil {
		return "", f.handoffErr
	}
	return f.handoffStr, nil
}

func (f *fakeAgentUsecase) RenameChat(
	_ context.Context,
	chatID, title, source string,
) error {
	f.renameCalls = append(f.renameCalls, renameCall{chatID: chatID, title: title, source: source})
	return f.renameErr
}

func (f *fakeAgentUsecase) RenameByRunner(
	_ context.Context,
	runnerID, title, source string,
) error {
	f.renameByRunnerCalls = append(f.renameByRunnerCalls, renameByRunnerCall{runnerID: runnerID, title: title, source: source})
	return f.renameByRunnerErr
}

func (f *fakeAgentUsecase) PurgeChat(
	_ context.Context,
	chatID string,
) error {
	f.purgeCalls = append(f.purgeCalls, chatID)
	return f.purgeErr
}

func (f *fakeAgentUsecase) ListProviders(
	_ context.Context,
	workspaceID string,
) ([]engineagent.Descriptor, error) {
	f.listProvidersWorkspace = workspaceID
	return f.providers, f.providersErr
}
