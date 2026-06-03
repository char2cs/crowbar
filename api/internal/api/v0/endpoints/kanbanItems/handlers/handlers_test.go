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

	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/kanbanItems/handlers"
)

var errBoom = errors.New("boom")

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ---------- mock ----------

type mockSvc struct {
	listFn func(ctx context.Context, taskID string) ([]domain.KanbanItem, error)
}

func (m *mockSvc) List(ctx context.Context, taskID string) ([]domain.KanbanItem, error) {
	return m.listFn(ctx, taskID)
}
func (m *mockSvc) Create(ctx context.Context, taskID, title string) (domain.KanbanItem, error) {
	return domain.KanbanItem{}, nil
}
func (m *mockSvc) UpdateStatus(ctx context.Context, id, status string) error { return nil }

func newRouter(h *handlers.Handlers) *gin.Engine {
	r := gin.New()
	r.GET("/tasks/:id/kanban-items", h.List)
	return r
}

// ---------- List ----------

func TestList_Happy(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, taskID string) ([]domain.KanbanItem, error) {
			return []domain.KanbanItem{{ID: "ki1", TaskID: taskID}}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/kanban-items", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ki1"`)
}

func TestList_SvcError(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, taskID string) ([]domain.KanbanItem, error) {
			return nil, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/kanban-items", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
