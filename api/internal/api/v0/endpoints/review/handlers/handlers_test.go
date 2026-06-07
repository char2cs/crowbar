package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	reviewhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/review/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type stubReview struct {
	review     domain.BranchReview
	thread     domain.ReviewThread
	getErr     error
	stratErr   error
	openErr    error
	replyErr   error
	resolveErr error

	lastWsID     string
	lastStrategy gitdomain.MergeStrategy
	lastOpen     branchreview.OpenThreadInput
	lastThreadID string
	lastReply    string
	lastResolved bool
}

func (s *stubReview) Get(
	_ context.Context,
	wsID string,
) (domain.BranchReview, error) {
	s.lastWsID = wsID
	return s.review, s.getErr
}

func (s *stubReview) SetMergeStrategy(
	_ context.Context,
	wsID string,
	strategy gitdomain.MergeStrategy,
) error {
	s.lastWsID = wsID
	s.lastStrategy = strategy
	return s.stratErr
}

func (s *stubReview) OpenThread(
	_ context.Context,
	in branchreview.OpenThreadInput,
) (domain.ReviewThread, error) {
	s.lastOpen = in
	return s.thread, s.openErr
}

func (s *stubReview) Reply(
	_ context.Context,
	threadID string,
	body string,
) (domain.ReviewThread, error) {
	s.lastThreadID = threadID
	s.lastReply = body
	return s.thread, s.replyErr
}

func (s *stubReview) SetThreadResolved(
	_ context.Context,
	threadID string,
	resolved bool,
) (domain.ReviewThread, error) {
	s.lastThreadID = threadID
	s.lastResolved = resolved
	return s.thread, s.resolveErr
}

type envelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func newRouter(
	stub *stubReview,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/v0")
	h := reviewhandlers.New(stub)
	g.GET("/workspaces/:wsId/review", h.Get)
	g.PATCH("/workspaces/:wsId/review", h.SetMergeStrategy)
	g.POST("/workspaces/:wsId/review/threads", h.OpenThread)
	g.POST("/workspaces/:wsId/review/threads/:id/reply", h.Reply)
	g.PATCH("/workspaces/:wsId/review/threads/:id", h.SetThreadResolved)
	return r
}

func do(
	t *testing.T,
	r *gin.Engine,
	method string,
	path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes(t, body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func bodyBytes(
	t *testing.T,
	body any,
) []byte {
	t.Helper()
	if raw, ok := body.([]byte); ok {
		return raw
	}
	if body == nil {
		return nil
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return raw
}

func decode(
	t *testing.T,
	w *httptest.ResponseRecorder,
) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

func jsonUnmarshal(
	raw json.RawMessage,
	v any,
) error {
	return json.Unmarshal(raw, v)
}

func assertAnError() error {
	return errors.New("boom")
}

func TestNew_BuildsHandlers(t *testing.T) {
	require.NotNil(t, reviewhandlers.New(&stubReview{}))
}

func TestEnvelope_BadJSON_NotSuccess(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review", []byte("{"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	env := decode(t, w)
	require.False(t, env.Success)
	require.NotEmpty(t, env.Error)
}
