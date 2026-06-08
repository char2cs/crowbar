package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agentrun/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubRepo struct{ run domain.AgentRun }

func (s stubRepo) Create(_ context.Context, id, wsID, chatID string, _ time.Time) (domain.AgentRun, error) {
	r := s.run
	r.ID = id
	r.WsID = wsID
	r.ChatID = chatID
	return r, nil
}
func (s stubRepo) ListRunning(_ context.Context) ([]domain.AgentRun, error) {
	return []domain.AgentRun{s.run}, nil
}
func (s stubRepo) MarkRunning(_ context.Context, id string) (domain.AgentRun, error) {
	r := s.run
	r.ID = id
	return r, nil
}
func (s stubRepo) Complete(_ context.Context, id string) (domain.AgentRun, error) {
	r := s.run
	r.ID = id
	return r, nil
}
func (s stubRepo) Fail(_ context.Context, id string) (domain.AgentRun, error) {
	r := s.run
	r.ID = id
	return r, nil
}
func (s stubRepo) Get(_ context.Context, id string) (domain.AgentRun, error) {
	r := s.run
	r.ID = id
	return r, nil
}

func newRouter(
	repo handlers.AgentRunRepo,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(repo)
	rg := r.Group("/v0")
	rg.POST("/workspaces/:wsId/runs", h.Create)
	rg.GET("/runs/running", h.ListRunning)
	rg.POST("/runs/:id/start", h.Start)
	rg.POST("/runs/:id/complete", h.Complete)
	rg.POST("/runs/:id/fail", h.Fail)
	rg.GET("/runs/:id", h.Get)
	return r
}

func do(
	r *gin.Engine,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestAgentRunHandlers_HappyPath(
	t *testing.T,
) {
	repo := stubRepo{run: domain.AgentRun{ID: "r1", WsID: "ws1", ChatID: "c1"}}
	r := newRouter(repo)

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/runs", map[string]any{"chatId": "c1"})
	require.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodGet, "/v0/runs/running", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/runs/r1/start", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/runs/r1/complete", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/runs/r1/fail", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodGet, "/v0/runs/r1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}
