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
	"github.com/char2cs/crowbar/api/internal/domain"
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

	switchCalls    []switchCall
	switchNewSegID string
	switchErr      error

	handoffStr string
	handoffErr error
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

func (f *fakeAgentUsecase) SpawnChat(
	_ context.Context,
	_ string,
	_ string,
) (string, string, error) {
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

func (f *fakeAgentUsecase) ListChats(
	_ context.Context,
) ([]domain.AgentChat, error) {
	return nil, nil
}

func (f *fakeAgentUsecase) GetChat(
	_ context.Context,
	_ string,
) (domain.AgentChat, error) {
	return domain.AgentChat{}, nil
}

func (f *fakeAgentUsecase) SegmentsFor(
	_ context.Context,
	_ string,
) ([]domain.AgentSegment, error) {
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

func (f *fakeAgentUsecase) AssembleHandoff(
	_ context.Context,
	_ string,
) (string, error) {
	if f.handoffErr != nil {
		return "", f.handoffErr
	}
	return f.handoffStr, nil
}
