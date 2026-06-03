package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/reviewThreads/handlers"
)

var errBoom = errors.New("boom")

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ---------- mock ----------

type mockSvc struct {
	getFn          func(ctx context.Context, threadID string) (domain.ReviewThread, error)
	listFn         func(ctx context.Context, taskID, status, phase string) ([]domain.ReviewThread, error)
	openFn         func(ctx context.Context, taskID, file string, line int, content string) (domain.ReviewThread, error)
	postMessageFn  func(ctx context.Context, threadID, role, content string) error
	forceApproveFn func(ctx context.Context, threadID string) error
}

func (m *mockSvc) Get(ctx context.Context, threadID string) (domain.ReviewThread, error) {
	if m.getFn != nil {
		return m.getFn(ctx, threadID)
	}
	return domain.ReviewThread{}, nil
}
func (m *mockSvc) List(ctx context.Context, taskID, status, phase string) ([]domain.ReviewThread, error) {
	return m.listFn(ctx, taskID, status, phase)
}
func (m *mockSvc) Open(ctx context.Context, taskID, file string, line int, content string) (domain.ReviewThread, error) {
	return m.openFn(ctx, taskID, file, line, content)
}
func (m *mockSvc) PostMessage(ctx context.Context, threadID, role, content string) error {
	return m.postMessageFn(ctx, threadID, role, content)
}
func (m *mockSvc) ForceApprove(ctx context.Context, threadID string) error {
	return m.forceApproveFn(ctx, threadID)
}

func newRouter(h *handlers.Handlers) *gin.Engine {
	r := gin.New()
	r.GET("/tasks/:id/threads", h.List)
	r.POST("/tasks/:id/threads", h.Open)
	r.POST("/threads/:id/messages", h.PostMessage)
	r.POST("/threads/:id/force-approve", h.ForceApprove)
	return r
}

func sampleThread(id string) domain.ReviewThread {
	return domain.ReviewThread{ID: id, TaskID: "t1", Status: domain.ReviewThreadStatusOpen}
}

// ---------- List ----------

func TestList_Happy(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, taskID, status, phase string) ([]domain.ReviewThread, error) {
			return []domain.ReviewThread{sampleThread("th1")}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/threads", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"th1"`)
}

func TestList_SvcError(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, taskID, status, phase string) ([]domain.ReviewThread, error) {
			return nil, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/threads", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Open ----------

func TestOpen_Happy(t *testing.T) {
	svc := &mockSvc{
		openFn: func(_ context.Context, taskID, file string, line int, content string) (domain.ReviewThread, error) {
			return sampleThread("th1"), nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	body := `{"file_path":"main.go","line_number":10,"content":"looks wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/threads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestOpen_BadRequest(t *testing.T) {
	r := newRouter(handlers.New(&mockSvc{}))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/threads", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOpen_SvcError(t *testing.T) {
	svc := &mockSvc{
		openFn: func(_ context.Context, taskID, file string, line int, content string) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	body := `{"file_path":"main.go","line_number":10,"content":"looks wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/threads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- PostMessage ----------

func TestPostMessage_BadRequest(t *testing.T) {
	r := newRouter(handlers.New(&mockSvc{}))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/th1/messages", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPostMessage_Happy(t *testing.T) {
	svc := &mockSvc{
		postMessageFn: func(_ context.Context, threadID, role, content string) error { return nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/th1/messages", strings.NewReader(`{"content":"reply"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestPostMessage_NotFound(t *testing.T) {
	svc := &mockSvc{
		postMessageFn: func(_ context.Context, threadID, role, content string) error {
			return repositories.ErrNotFound
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/nope/messages", strings.NewReader(`{"content":"reply"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPostMessage_SvcError(t *testing.T) {
	svc := &mockSvc{
		postMessageFn: func(_ context.Context, threadID, role, content string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/th1/messages", strings.NewReader(`{"content":"reply"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- ForceApprove ----------

func TestForceApprove_Happy(t *testing.T) {
	svc := &mockSvc{
		forceApproveFn: func(_ context.Context, threadID string) error { return nil },
		getFn: func(_ context.Context, threadID string) (domain.ReviewThread, error) {
			return sampleThread(threadID), nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/th1/force-approve", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestForceApprove_NotFound(t *testing.T) {
	svc := &mockSvc{
		forceApproveFn: func(_ context.Context, threadID string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/nope/force-approve", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestForceApprove_SvcError(t *testing.T) {
	svc := &mockSvc{
		forceApproveFn: func(_ context.Context, threadID string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/th1/force-approve", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestForceApprove_GetError(t *testing.T) {
	svc := &mockSvc{
		forceApproveFn: func(_ context.Context, threadID string) error { return nil },
		getFn: func(_ context.Context, threadID string) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/threads/th1/force-approve", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
