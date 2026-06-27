package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ── HomeReader mock ────────────────────────────────────────────────────────────

type mockHomeReader struct{ mock.Mock }

func (m *mockHomeReader) GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error) {
	args := m.Called(ctx, projectID)
	return args.Get(0).(domain.Workspace), args.Error(1)
}

// ── Files stub (no-op, always returns empty results) ──────────────────────────

type stubFiles struct{}

func (s *stubFiles) Tree(_ context.Context, _ string, _ string, _ fileusecase.FileStatusProvider) ([]domain.FileNode, error) {
	return []domain.FileNode{}, nil
}
func (s *stubFiles) ReadContent(_ context.Context, _ string, _ string) (domain.FileContent, error) {
	return domain.FileContent{}, nil
}
func (s *stubFiles) WriteContent(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (s *stubFiles) CreateFile(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (s *stubFiles) CreateDir(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (s *stubFiles) Rename(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (s *stubFiles) Delete(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestGetHome_Returns200WithWorkspace verifies that a GET /home on an existing
// project returns HTTP 200 and calls GetHomeForProject exactly once.
func TestGetHome_Returns200WithWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	homeWS := domain.Workspace{
		ID:           "ws-home-1",
		ProjectID:    "proj-1",
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/myproject",
	}
	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-1").Return(homeWS, nil)

	h := handlers.New(reader, nil, nil)
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-1/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	reader.AssertExpectations(t)
}

// TestGetHome_Returns404WhenNotFound verifies that a GET /home for a project
// whose home workspace is missing returns HTTP 404.
func TestGetHome_Returns404WhenNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-missing").
		Return(domain.Workspace{}, apperr.ErrNotFound)

	h := handlers.New(reader, nil, nil)
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-missing/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	reader.AssertExpectations(t)
}

// TestGetHome_Returns500OnStorageError verifies that a GET /home when storage
// returns an unexpected error (not ErrNotFound) returns HTTP 500, not 404.
func TestGetHome_Returns500OnStorageError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-err").
		Return(domain.Workspace{}, errors.New("asynx: read failed"))

	h := handlers.New(reader, nil, nil)
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-err/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	reader.AssertExpectations(t)
}

// TestFileTree_Returns200WhenWorkspaceExists verifies that GET /home/files/tree
// returns HTTP 200 when the home workspace is found.
func TestFileTree_Returns200WhenWorkspaceExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	homeWS := domain.Workspace{
		ID:           "ws-home-2",
		ProjectID:    "proj-2",
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/myproject2",
	}
	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-2").Return(homeWS, nil)

	h := handlers.New(reader, &stubFiles{}, nil)
	r.GET("/projects/:projectId/home/files/tree", h.FileTree)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-2/home/files/tree", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	reader.AssertExpectations(t)
}
