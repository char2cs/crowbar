package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agentRuns/handlers"
)

var errBoom = errors.New("boom")

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ---------- mock ----------

type mockSvc struct {
	listFn      func(ctx context.Context, taskID string) ([]domain.AgentRun, error)
	interruptFn func(ctx context.Context, id string) error
}

func (m *mockSvc) List(ctx context.Context, taskID string) ([]domain.AgentRun, error) {
	return m.listFn(ctx, taskID)
}
func (m *mockSvc) Interrupt(ctx context.Context, id string) error {
	return m.interruptFn(ctx, id)
}

func newRouter(h *handlers.Handlers) *gin.Engine {
	r := gin.New()
	r.GET("/tasks/:id/agent-runs", h.List)
	r.POST("/agent-runs/:id/interrupt", h.Interrupt)
	return r
}

func sampleRun(id string) domain.AgentRun {
	return domain.AgentRun{ID: id, TaskID: "t1", StateName: "implementing", Status: domain.AgentRunStatusRunning}
}

// ---------- List ----------

func TestList_Happy(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, taskID string) ([]domain.AgentRun, error) {
			return []domain.AgentRun{sampleRun("ar1")}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/agent-runs", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ar1"`)
}

func TestList_SvcError(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, taskID string) ([]domain.AgentRun, error) {
			return nil, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/agent-runs", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Interrupt ----------

func TestInterrupt_Happy(t *testing.T) {
	svc := &mockSvc{
		interruptFn: func(_ context.Context, id string) error { return nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agent-runs/ar1/interrupt", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestInterrupt_NotFound(t *testing.T) {
	svc := &mockSvc{
		interruptFn: func(_ context.Context, id string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agent-runs/nope/interrupt", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestInterrupt_SvcError(t *testing.T) {
	svc := &mockSvc{
		interruptFn: func(_ context.Context, id string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agent-runs/ar1/interrupt", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
