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

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
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
	h := newChatHandlers(uc)

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
	h := newChatHandlers(uc)

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
	h := newChatHandlers(uc)

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
	// terminalWait is the standing "is this chat's CLI parked on a modal we
	// cannot answer" verdict. Zero — not waiting — for every test that does not
	// set it, which is the state a chat is in unless something says otherwise.
	terminalWait domain.AgentTerminalWait

	switchCalls    []switchCall
	switchNewSegID string
	switchErr      error

	resumeCalls []string
	resumeSegID string
	resumeErr   error

	stopCalls []string
	stopErr   error

	handoffStr string
	handoffErr error

	renameCalls []renameCall
	renameErr   error

	selectionCalls []selectionCall
	selectionErr   error

	promptAwaitCalls  []promptAwaitCall
	promptAwaitPrompt string
	promptAwaitFound  bool
	promptAwaitAcked  bool
	promptAwaitErr    error

	dispatchMCPCalls []dispatchMCPCall
	dispatchMCPOut   []byte
	dispatchMCPSend  bool
	dispatchMCPErr   error

	purgeCalls []string
	purgeErr   error

	resolveProviders []dto.AgentProviderDTO
	resolveErr       error

	replaceResult []dto.AgentProviderDTO
	replaceErr    error
	replaceCalls  [][]domain.AgentProviderPreference

	// getChat/getChatErr configure GetChat, the call every
	// requireChatInWorkspace scope check (Get/Switch/Rename/Handoff) makes
	// first. The zero value (an empty domain.AgentChat, WorkspaceID "") makes
	// the scope check pass by default for callers that leave :wsId unset too.
	getChat    domain.AgentChat
	getChatErr error

	activity      agentusecase.ChatActivity
	activityErr   error
	activityCalls []activityCall
	payload       []byte
	payloadErr    error
	payloadCalls  []payloadCall
	pending       []domain.ActivityChoice
	pendingErr    error
	pendingCalls  []string

	answerable    []string
	answerCalls   []answerCall
	answerErr     error
	pendingAnswer agentusecase.PendingAnswer
	pendingAwait  bool
	awaitAnswer   agentusecase.HookAnswer
	awaitErr      error
	awaitCalls    []string
	abandonCalls  []string
	abandonErr    error
	telemetry     engineagents.Telemetry
	telemetryOK   bool

	messagePage  chatlog.Page
	messageErr   error
	messageCalls []messageCall
	promptResult dto.PromptSubmissionDTO
	promptErr    error
	promptCalls  []promptCall
	catalog      engineagents.SlashCatalog
	catalogErr   error
	catalogCalls []string
}

type promptCall struct {
	chatID, text, requestID string
}

type messageCall struct {
	chatID               string
	after, before, limit int
}

type ingestCall struct {
	segID    string
	provider string
	event    string
	raw      []byte
}

type switchCall struct {
	chatID   string
	provider string
}

type selectionCall struct {
	chatID string
	model  string
	effort string
}

type renameCall struct {
	chatID string
	title  string
	source string
}

// promptAwaitCall records one prompt-collector poll, so a handler test can assert
// what the runner-keyed route actually forwarded.
type promptAwaitCall struct {
	runnerID string
	token    string
	waitMS   int64
}

type dispatchMCPCall struct {
	runnerID string
	token    string
	message  []byte
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

func (f *fakeAgentUsecase) ReadMessages(
	_ context.Context,
	chatID string,
	after, before, limit int,
) (chatlog.Page, error) {
	f.messageCalls = append(f.messageCalls, messageCall{chatID: chatID, after: after, before: before, limit: limit})
	return f.messagePage, f.messageErr
}

func (f *fakeAgentUsecase) SubmitPrompt(
	_ context.Context,
	chatID, text, requestID string,
) (dto.PromptSubmissionDTO, error) {
	f.promptCalls = append(f.promptCalls, promptCall{chatID: chatID, text: text, requestID: requestID})
	return f.promptResult, f.promptErr
}

func (f *fakeAgentUsecase) SlashCatalog(
	_ context.Context,
	chatID string,
) (engineagents.SlashCatalog, error) {
	f.catalogCalls = append(f.catalogCalls, chatID)
	return f.catalog, f.catalogErr
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

func (f *fakeAgentUsecase) StopChat(
	_ context.Context,
	chatID string,
) error {
	f.stopCalls = append(f.stopCalls, chatID)
	return f.stopErr
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

func (f *fakeAgentUsecase) SetChatSelection(
	_ context.Context,
	chatID, model, effort string,
) error {
	f.selectionCalls = append(f.selectionCalls, selectionCall{chatID: chatID, model: model, effort: effort})
	return f.selectionErr
}

func (f *fakeAgentUsecase) AwaitQueuedPrompt(
	_ context.Context,
	runnerID, token string,
	waitMS int64,
) (string, bool, func(), error) {
	f.promptAwaitCalls = append(f.promptAwaitCalls,
		promptAwaitCall{runnerID: runnerID, token: token, waitMS: waitMS})
	if f.promptAwaitErr != nil {
		return "", false, func() {}, f.promptAwaitErr
	}
	return f.promptAwaitPrompt, f.promptAwaitFound, func() { f.promptAwaitAcked = true }, nil
}

func (f *fakeAgentUsecase) DispatchMCP(
	_ context.Context,
	runnerID, token string,
	message []byte,
) ([]byte, bool, error) {
	f.dispatchMCPCalls = append(f.dispatchMCPCalls, dispatchMCPCall{
		runnerID: runnerID,
		token:    token,
		message:  message,
	})
	if f.dispatchMCPErr != nil {
		return nil, false, f.dispatchMCPErr
	}
	return f.dispatchMCPOut, f.dispatchMCPSend, nil
}

func (f *fakeAgentUsecase) PurgeChat(
	_ context.Context,
	chatID string,
) error {
	f.purgeCalls = append(f.purgeCalls, chatID)
	return f.purgeErr
}

func (f *fakeAgentUsecase) ResolveProviders(
	_ context.Context,
) ([]dto.AgentProviderDTO, error) {
	return f.resolveProviders, f.resolveErr
}

func (f *fakeAgentUsecase) ReplaceProviderPreferences(
	_ context.Context,
	prefs []domain.AgentProviderPreference,
) ([]dto.AgentProviderDTO, error) {
	f.replaceCalls = append(f.replaceCalls, prefs)
	if f.replaceErr != nil {
		return nil, f.replaceErr
	}
	return f.replaceResult, nil
}

func (f *fakeAgentUsecase) ReadActivity(
	_ context.Context, chatID string, after int64, limit int,
) (agentusecase.ChatActivity, error) {
	f.activityCalls = append(f.activityCalls, activityCall{chatID: chatID, after: after, limit: limit})
	return f.activity, f.activityErr
}

func (f *fakeAgentUsecase) ReadToolPayload(
	_ context.Context, chatID, toolID, side string,
) ([]byte, error) {
	f.payloadCalls = append(f.payloadCalls, payloadCall{chatID: chatID, toolID: toolID, side: side})
	return f.payload, f.payloadErr
}

func (f *fakeAgentUsecase) ReadPendingChoices(
	_ context.Context, chatID string,
) ([]domain.ActivityChoice, error) {
	f.pendingCalls = append(f.pendingCalls, chatID)
	return f.pending, f.pendingErr
}

func (f *fakeAgentUsecase) TerminalWait(string) domain.AgentTerminalWait {
	return f.terminalWait
}

func (f *fakeAgentUsecase) AnswerableChoiceIDs(string, []domain.ActivityChoice) []string {
	return f.answerable
}

func (f *fakeAgentUsecase) AnswerChoice(
	_ context.Context, chatID, choiceID string, optionIDs []string, reason string, content []byte,
) error {
	f.answerCalls = append(f.answerCalls, answerCall{
		chatID: chatID, choiceID: choiceID, optionIDs: optionIDs,
		reason: reason, content: string(content),
	})
	return f.answerErr
}

func (f *fakeAgentUsecase) PendingAnswer(string) (agentusecase.PendingAnswer, bool) {
	return f.pendingAnswer, f.pendingAwait
}

func (f *fakeAgentUsecase) AwaitAnswer(
	_ context.Context, deliveryID string,
) (agentusecase.HookAnswer, error) {
	f.awaitCalls = append(f.awaitCalls, deliveryID)
	return f.awaitAnswer, f.awaitErr
}

func (f *fakeAgentUsecase) AbandonAnswer(_ context.Context, deliveryID string) error {
	f.abandonCalls = append(f.abandonCalls, deliveryID)
	return f.abandonErr
}

// answerCall records one AnswerChoice, so a handler test can assert that what a
// client sent is what the usecase was told — the option ids especially, since
// they are the only thing that decides which provider template gets rendered.
type answerCall struct {
	chatID    string
	choiceID  string
	optionIDs []string
	reason    string
	content   string
}

func (f *fakeAgentUsecase) Telemetry(string) (engineagents.Telemetry, bool) {
	return f.telemetry, f.telemetryOK
}

type activityCall struct {
	chatID string
	after  int64
	limit  int
}

type payloadCall struct {
	chatID, toolID, side string
}
