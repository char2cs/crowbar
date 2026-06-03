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

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/tasks/handlers"
)

var errBoom = errors.New("boom")

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ---------- mock ----------

type mockSvc struct {
	createFn          func(ctx context.Context, repositoryID, title, branchName, baseBranch, flowPath string) (domain.Task, error)
	getFn             func(ctx context.Context, id string) (domain.Task, error)
	listFn            func(ctx context.Context, repositoryID string) ([]domain.Task, error)
	archiveFn         func(ctx context.Context, id string) error
	pauseFn           func(ctx context.Context, id string) error
	resumeFn          func(ctx context.Context, id string) error
	retryFn           func(ctx context.Context, id string) error
	forceTransitionFn func(ctx context.Context, id, state string) error
}

func (m *mockSvc) Create(ctx context.Context, repositoryID, title, branchName, baseBranch, flowPath string) (domain.Task, error) {
	return m.createFn(ctx, repositoryID, title, branchName, baseBranch, flowPath)
}
func (m *mockSvc) Get(ctx context.Context, id string) (domain.Task, error) {
	return m.getFn(ctx, id)
}
func (m *mockSvc) List(ctx context.Context, repositoryID string) ([]domain.Task, error) {
	return m.listFn(ctx, repositoryID)
}
func (m *mockSvc) Archive(ctx context.Context, id string) error   { return m.archiveFn(ctx, id) }
func (m *mockSvc) Pause(ctx context.Context, id string) error     { return m.pauseFn(ctx, id) }
func (m *mockSvc) Resume(ctx context.Context, id string) error    { return m.resumeFn(ctx, id) }
func (m *mockSvc) Retry(ctx context.Context, id string) error     { return m.retryFn(ctx, id) }
func (m *mockSvc) ForceTransition(ctx context.Context, id, state string) error {
	return m.forceTransitionFn(ctx, id, state)
}

func sampleTask(id string) domain.Task {
	return domain.Task{
		ID:           id,
		RepositoryID: "r1",
		Title:        "test task",
		Status:       domain.TaskStatusPending,
		BranchName:   "feat/x",
		BaseBranch:   "main",
		WorktreePath: "/tmp/wt",
	}
}

func newRouter(h *handlers.Handlers) *gin.Engine {
	r := gin.New()
	r.POST("/repositories/:id/tasks", h.Create)
	r.GET("/repositories/:id/tasks", h.List)
	r.GET("/tasks/:id", h.Get)
	r.POST("/tasks/:id/archive", h.Archive)
	r.POST("/tasks/:id/pause", h.Pause)
	r.POST("/tasks/:id/resume", h.Resume)
	r.POST("/tasks/:id/retry", h.Retry)
	r.POST("/tasks/:id/force-transition", h.ForceTransition)
	return r
}

// ---------- List ----------

func TestList_Happy(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, repositoryID string) ([]domain.Task, error) {
			return []domain.Task{sampleTask("t1"), sampleTask("t2")}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/repositories/r1/tasks", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestList_SvcError(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context, repositoryID string) ([]domain.Task, error) {
			return nil, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/repositories/r1/tasks", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Create ----------

func TestCreate_Happy(t *testing.T) {
	svc := &mockSvc{
		createFn: func(_ context.Context, repositoryID, title, branchName, baseBranch, flowPath string) (domain.Task, error) {
			return sampleTask("t1"), nil
		},
	}
	r := newRouter(handlers.New(svc))
	body := `{"title":"my task","branch_name":"feat/x","base_branch":"main"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repositories/r1/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreate_BadRequest(t *testing.T) {
	r := newRouter(handlers.New(&mockSvc{}))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repositories/r1/tasks", strings.NewReader(`{"title":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreate_RepoNotFound(t *testing.T) {
	svc := &mockSvc{
		createFn: func(_ context.Context, repositoryID, title, branchName, baseBranch, flowPath string) (domain.Task, error) {
			return domain.Task{}, repositories.ErrNotFound
		},
	}
	r := newRouter(handlers.New(svc))
	body := `{"title":"x","branch_name":"b","base_branch":"main"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repositories/r1/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCreate_SvcError(t *testing.T) {
	svc := &mockSvc{
		createFn: func(_ context.Context, repositoryID, title, branchName, baseBranch, flowPath string) (domain.Task, error) {
			return domain.Task{}, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	body := `{"title":"x","branch_name":"b","base_branch":"main"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/repositories/r1/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Get ----------

func TestGet_Happy(t *testing.T) {
	svc := &mockSvc{
		getFn: func(_ context.Context, id string) (domain.Task, error) { return sampleTask(id), nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGet_NotFound(t *testing.T) {
	svc := &mockSvc{
		getFn: func(_ context.Context, id string) (domain.Task, error) {
			return domain.Task{}, repositories.ErrNotFound
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/nope", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGet_SvcError(t *testing.T) {
	svc := &mockSvc{
		getFn: func(_ context.Context, id string) (domain.Task, error) { return domain.Task{}, errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Archive ----------

func TestArchive_Happy(t *testing.T) {
	svc := &mockSvc{
		archiveFn: func(_ context.Context, id string) error { return nil },
		getFn:     func(_ context.Context, id string) (domain.Task, error) { return sampleTask(id), nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/archive", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestArchive_NotFound(t *testing.T) {
	svc := &mockSvc{
		archiveFn: func(_ context.Context, id string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/nope/archive", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestArchive_SvcError(t *testing.T) {
	svc := &mockSvc{
		archiveFn: func(_ context.Context, id string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/archive", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- ForceTransition ----------

func TestForceTransition_Happy(t *testing.T) {
	svc := &mockSvc{
		forceTransitionFn: func(_ context.Context, id, state string) error { return nil },
		getFn:             func(_ context.Context, id string) (domain.Task, error) { return sampleTask(id), nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/force-transition", strings.NewReader(`{"to_state":"planning"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestForceTransition_BadRequest(t *testing.T) {
	r := newRouter(handlers.New(&mockSvc{}))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/force-transition", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestForceTransition_NotFound(t *testing.T) {
	svc := &mockSvc{
		forceTransitionFn: func(_ context.Context, id, state string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/nope/force-transition", strings.NewReader(`{"to_state":"planning"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestForceTransition_SvcError(t *testing.T) {
	svc := &mockSvc{
		forceTransitionFn: func(_ context.Context, id, state string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/force-transition", strings.NewReader(`{"to_state":"planning"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestForceTransition_GetError(t *testing.T) {
	svc := &mockSvc{
		forceTransitionFn: func(_ context.Context, id, state string) error { return nil },
		getFn:             func(_ context.Context, id string) (domain.Task, error) { return domain.Task{}, errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/force-transition", strings.NewReader(`{"to_state":"planning"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Archive (get-after-archive error) ----------

func TestArchive_GetError(t *testing.T) {
	svc := &mockSvc{
		archiveFn: func(_ context.Context, id string) error { return nil },
		getFn:     func(_ context.Context, id string) (domain.Task, error) { return domain.Task{}, errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/archive", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Pause ----------

func TestPause_Happy(t *testing.T) {
	svc := &mockSvc{
		pauseFn: func(_ context.Context, id string) error { return nil },
		getFn:   func(_ context.Context, id string) (domain.Task, error) { return sampleTask(id), nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/pause", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPause_NotFound(t *testing.T) {
	svc := &mockSvc{
		pauseFn: func(_ context.Context, id string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/nope/pause", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPause_SvcError(t *testing.T) {
	svc := &mockSvc{
		pauseFn: func(_ context.Context, id string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/pause", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPause_GetError(t *testing.T) {
	svc := &mockSvc{
		pauseFn: func(_ context.Context, id string) error { return nil },
		getFn:   func(_ context.Context, id string) (domain.Task, error) { return domain.Task{}, errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/pause", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Resume ----------

func TestResume_Happy(t *testing.T) {
	svc := &mockSvc{
		resumeFn: func(_ context.Context, id string) error { return nil },
		getFn:    func(_ context.Context, id string) (domain.Task, error) { return sampleTask(id), nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/resume", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestResume_NotFound(t *testing.T) {
	svc := &mockSvc{
		resumeFn: func(_ context.Context, id string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/nope/resume", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResume_SvcError(t *testing.T) {
	svc := &mockSvc{
		resumeFn: func(_ context.Context, id string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/resume", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestResume_GetError(t *testing.T) {
	svc := &mockSvc{
		resumeFn: func(_ context.Context, id string) error { return nil },
		getFn:    func(_ context.Context, id string) (domain.Task, error) { return domain.Task{}, errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/resume", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Retry ----------

func TestRetry_Happy(t *testing.T) {
	svc := &mockSvc{
		retryFn: func(_ context.Context, id string) error { return nil },
		getFn:   func(_ context.Context, id string) (domain.Task, error) { return sampleTask(id), nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/retry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRetry_NotFound(t *testing.T) {
	svc := &mockSvc{
		retryFn: func(_ context.Context, id string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/nope/retry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRetry_SvcError(t *testing.T) {
	svc := &mockSvc{
		retryFn: func(_ context.Context, id string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/retry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRetry_GetError(t *testing.T) {
	svc := &mockSvc{
		retryFn: func(_ context.Context, id string) error { return nil },
		getFn:   func(_ context.Context, id string) (domain.Task, error) { return domain.Task{}, errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tasks/t1/retry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
