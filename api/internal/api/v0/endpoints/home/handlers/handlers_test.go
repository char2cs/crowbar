package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type mockHomeReader struct{ mock.Mock }

func (m *mockHomeReader) GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error) {
	args := m.Called(ctx, projectID)
	return args.Get(0).(domain.Workspace), args.Error(1)
}

type stubFiles struct{}

func (s *stubFiles) Tree(_ context.Context, _, _ string, _ fileusecase.FileStatusProvider) ([]domain.FileNode, error) {
	return []domain.FileNode{}, nil
}

func (s *stubFiles) CreateFile(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) CreateDir(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) Copy(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) Rename(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) Delete(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

type stubWork struct{ working bool }

func (s stubWork) WorkingFor(_ string) bool { return s.working }

func TestGetHome_StampsWorkingFromSignal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-1").
		Return(domain.Workspace{ID: "ws-home-1", ProjectID: "proj-1", Kind: domain.WorkspaceKindHome}, nil)

	h := handlers.New(reader, nil, stubWork{working: true})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-1/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			ID      string `json:"id"`
			Working bool   `json:"working"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "ws-home-1", body.Data.ID)
	require.True(t, body.Data.Working, "GET /home must stamp the derived working overlay")
}

func TestGetHome_Returns200WithWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	homeWS := domain.Workspace{
		ID:           "ws-home-1",
		ProjectID:    "proj-1",
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/myproject",
		Provisioning: domain.WorkspaceShared,
	}
	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-1").Return(homeWS, nil)

	h := handlers.New(reader, nil, stubWork{})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-1/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	reader.AssertExpectations(t)
}

func TestGetHome_Returns404WhenNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-missing").
		Return(domain.Workspace{}, problem.ErrNotFound)

	h := handlers.New(reader, nil, stubWork{})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-missing/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	reader.AssertExpectations(t)
}

func TestGetHome_Returns500OnStorageError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-err").
		Return(domain.Workspace{}, errors.New("asynx: read failed"))

	h := handlers.New(reader, nil, stubWork{})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-err/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	reader.AssertExpectations(t)
}

func TestFileTree_Returns200WhenWorkspaceExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	homeWS := domain.Workspace{
		ID:           "ws-home-2",
		ProjectID:    "proj-2",
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/myproject2",
		Provisioning: domain.WorkspaceShared,
	}
	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-2").Return(homeWS, nil)

	h := handlers.New(reader, &stubFiles{}, stubWork{})
	r.GET("/projects/:projectId/home/files/tree", h.FileTree)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-2/home/files/tree", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	reader.AssertExpectations(t)
}

func TestRequireHomeWorkspace_SetsWsIdAndCallsNext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-mw").Return(domain.Workspace{
		ID:        "ws-mw",
		ProjectID: "proj-mw",
		Kind:      domain.WorkspaceKindHome,
	}, nil)

	h := handlers.New(reader, nil, stubWork{})
	var capturedWsID string
	r.GET("/projects/:projectId/home/thing", h.RequireHomeWorkspace, func(c *gin.Context) {
		capturedWsID = c.Param("wsId")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-mw/home/thing", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "ws-mw", capturedWsID)
	reader.AssertExpectations(t)
}

func TestRequireHomeWorkspace_AbortsChainOnFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-abort").
		Return(domain.Workspace{}, errors.New("storage error"))

	h := handlers.New(reader, nil, stubWork{})
	called := false
	r.GET("/projects/:projectId/home/thing", h.RequireHomeWorkspace, func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-abort/home/thing", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.False(t, called)
	reader.AssertExpectations(t)
}

func missingFile(t *testing.T) error {
	t.Helper()
	_, err := os.Lstat(filepath.Join(t.TempDir(), "ghost.txt"))
	require.Error(t, err)
	return fmt.Errorf("file: copy: %w", err)
}
