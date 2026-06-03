package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git/handlers"
)

var errBoom = errors.New("boom")

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ---------- mock ----------

type mockSvc struct {
	logFn       func(ctx context.Context, taskID string) ([]domain.GitCommit, error)
	diffFn      func(ctx context.Context, taskID string) (string, error)
	listFilesFn func(ctx context.Context, taskID string) ([]string, error)
}

func (m *mockSvc) Log(ctx context.Context, taskID string) ([]domain.GitCommit, error) {
	return m.logFn(ctx, taskID)
}
func (m *mockSvc) Diff(ctx context.Context, taskID string) (string, error) {
	return m.diffFn(ctx, taskID)
}
func (m *mockSvc) ListFiles(ctx context.Context, taskID string) ([]string, error) {
	return m.listFilesFn(ctx, taskID)
}

func newRouter(h *handlers.Handlers) *gin.Engine {
	r := gin.New()
	r.GET("/tasks/:id/git/log", h.Log)
	r.GET("/tasks/:id/git/diff", h.Diff)
	r.GET("/tasks/:id/files", h.ListFiles)
	return r
}

// ---------- Log ----------

func TestLog_Happy(t *testing.T) {
	svc := &mockSvc{
		logFn: func(_ context.Context, taskID string) ([]domain.GitCommit, error) {
			return []domain.GitCommit{{SHA: "abc123", Message: "initial commit", Date: time.Now()}}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/git/log", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "initial commit")
}

func TestLog_TaskNotFound(t *testing.T) {
	svc := &mockSvc{
		logFn: func(_ context.Context, taskID string) ([]domain.GitCommit, error) {
			return nil, repositories.ErrNotFound
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/nope/git/log", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLog_SvcError(t *testing.T) {
	svc := &mockSvc{
		logFn: func(_ context.Context, taskID string) ([]domain.GitCommit, error) {
			return nil, errBoom
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/git/log", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Diff ----------

func TestDiff_Happy(t *testing.T) {
	rawDiff := "+++ b/hello.txt\n@@ -1 +1,2 @@\n+world\n"
	svc := &mockSvc{
		diffFn: func(_ context.Context, taskID string) (string, error) {
			return rawDiff, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/git/diff", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"files"`)
}

func TestDiff_EmptyDiff(t *testing.T) {
	svc := &mockSvc{
		diffFn: func(_ context.Context, taskID string) (string, error) { return "", nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/git/diff", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"files":[]`)
}

func TestDiff_TaskNotFound(t *testing.T) {
	svc := &mockSvc{
		diffFn: func(_ context.Context, taskID string) (string, error) {
			return "", repositories.ErrNotFound
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/nope/git/diff", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDiff_SvcError(t *testing.T) {
	svc := &mockSvc{
		diffFn: func(_ context.Context, taskID string) (string, error) { return "", errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/git/diff", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- ListFiles ----------

func TestListFiles_Happy(t *testing.T) {
	svc := &mockSvc{
		listFilesFn: func(_ context.Context, taskID string) ([]string, error) {
			return []string{"hello.txt", "main.go"}, nil
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/files", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "hello.txt")
}

func TestListFiles_TaskNotFound(t *testing.T) {
	svc := &mockSvc{
		listFilesFn: func(_ context.Context, taskID string) ([]string, error) {
			return nil, repositories.ErrNotFound
		},
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/nope/files", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListFiles_SvcError(t *testing.T) {
	svc := &mockSvc{
		listFilesFn: func(_ context.Context, taskID string) ([]string, error) { return nil, errBoom },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/files", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---------- Diff (parseUnifiedDiff edge cases) ----------

// TestDiff_HunkAndLinesBeforeFile exercises the @@ / +line / -line branches
// that are reached when currentFile is "" (no +++ header seen yet).
func TestDiff_HunkAndLinesBeforeFile(t *testing.T) {
	// The raw diff has a hunk header, an addition, and a removal all before
	// any "+++ b/" line, so currentFile remains "".  The parser must skip
	// them without panicking, and return an empty files list.
	rawDiff := "@@ -1 +1 @@\n+orphan add\n-orphan remove\n"
	svc := &mockSvc{
		diffFn: func(_ context.Context, taskID string) (string, error) { return rawDiff, nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/git/diff", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"files":[]`)
}

// TestDiff_MultipleFiles exercises the removal-line branch with a real file
// in scope to confirm stat.Removed is counted.
func TestDiff_MultipleFiles(t *testing.T) {
	rawDiff := "+++ b/a.go\n@@ -1,2 +1,1 @@\n-removed line\n+added line\n+++ b/b.go\n@@ -0,0 +1 @@\n+new file line\n"
	svc := &mockSvc{
		diffFn: func(_ context.Context, taskID string) (string, error) { return rawDiff, nil },
	}
	r := newRouter(handlers.New(svc))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks/t1/git/diff", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "a.go")
	assert.Contains(t, w.Body.String(), "b.go")
}
