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

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
)

var errBoom = errors.New("boom")

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ---------- mock ----------

type mockSvc struct {
	createFn func(ctx context.Context, name string) (domain.Project, error)
	listFn   func(ctx context.Context) ([]domain.Project, error)
	getFn    func(ctx context.Context, id string) (domain.Project, error)
	deleteFn func(ctx context.Context, id string) error
}

func (m *mockSvc) Create(ctx context.Context, name string) (domain.Project, error) {
	return m.createFn(ctx, name)
}
func (m *mockSvc) List(ctx context.Context) ([]domain.Project, error) {
	return m.listFn(ctx)
}
func (m *mockSvc) Get(ctx context.Context, id string) (domain.Project, error) {
	return m.getFn(ctx, id)
}
func (m *mockSvc) Delete(ctx context.Context, id string) error {
	return m.deleteFn(ctx, id)
}

func newRouter(h *handlers.Handlers) *gin.Engine {
	r := gin.New()
	r.POST("/projects", h.Create)
	r.GET("/projects", h.List)
	r.GET("/projects/:id", h.Get)
	r.DELETE("/projects/:id", h.Delete)
	return r
}

// ---------- Create ----------

func TestCreate_Happy(t *testing.T) {
	svc := &mockSvc{
		createFn: func(_ context.Context, name string) (domain.Project, error) {
			return domain.Project{ID: "p1", Name: name}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"p1"`)
}

func TestCreate_BadRequest(t *testing.T) {
	r := newRouter(handlers.New(&mockSvc{}))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreate_SvcError(t *testing.T) {
	svc := &mockSvc{
		createFn: func(_ context.Context, name string) (domain.Project, error) {
			return domain.Project{}, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- List ----------

func TestList_Happy(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context) ([]domain.Project, error) {
			return []domain.Project{{ID: "p1", Name: "a"}}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestList_SvcError(t *testing.T) {
	svc := &mockSvc{
		listFn: func(_ context.Context) ([]domain.Project, error) {
			return nil, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Get ----------

func TestGet_Happy(t *testing.T) {
	svc := &mockSvc{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{ID: id, Name: "x"}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/p1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGet_NotFound(t *testing.T) {
	svc := &mockSvc{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{}, repositories.ErrNotFound
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/nope", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGet_SvcError(t *testing.T) {
	svc := &mockSvc{
		getFn: func(_ context.Context, id string) (domain.Project, error) {
			return domain.Project{}, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/p1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Delete ----------

func TestDelete_Happy(t *testing.T) {
	svc := &mockSvc{
		deleteFn: func(_ context.Context, id string) error { return nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/projects/p1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDelete_NotFound(t *testing.T) {
	svc := &mockSvc{
		deleteFn: func(_ context.Context, id string) error { return repositories.ErrNotFound },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/projects/nope", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDelete_SvcError(t *testing.T) {
	svc := &mockSvc{
		deleteFn: func(_ context.Context, id string) error { return errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/projects/p1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
