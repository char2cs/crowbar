package handlers_test

import (
	"context"
	"encoding/json"
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

func (m *mockHomeReader) CreateHome(ctx context.Context, projectID, worktreePath string, now time.Time) (domain.Workspace, error) {
	args := m.Called(ctx, projectID, worktreePath, now)
	return args.Get(0).(domain.Workspace), args.Error(1)
}

// ── ProjectReader stub — always returns "not found" ──────────────────────────

type stubProjectReader struct{}

func (stubProjectReader) FindByKey(_ context.Context, _ string) (*domain.Project, error) {
	return nil, apperr.ErrNotFound
}

// ── ProjectReader mock — controllable for lazy home provisioning tests ───────

type mockProjectReader struct{ mock.Mock }

func (m *mockProjectReader) FindByKey(ctx context.Context, id string) (*domain.Project, error) {
	args := m.Called(ctx, id)
	proj, _ := args.Get(0).(*domain.Project)
	return proj, args.Error(1)
}

// ── Files stub (no-op, always returns empty results) ──────────────────────────

type stubFiles struct{}

func (s *stubFiles) Tree(_ context.Context, _, _ string, _ fileusecase.FileStatusProvider) ([]domain.FileNode, error) {
	return []domain.FileNode{}, nil
}

func (s *stubFiles) ReadContent(_ context.Context, _, _ string) (domain.FileContent, error) {
	return domain.FileContent{}, nil
}

func (s *stubFiles) WriteContent(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) CreateFile(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) CreateDir(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) Rename(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubFiles) Delete(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

// ── WorkSignal stub — the derived working overlay the home read stamps from ──

type stubWork struct{ working bool }

func (s stubWork) WorkingFor(_ string) bool { return s.working }

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestGetHome_StampsWorkingFromSignal pins that GET /home stamps the workspace's
// `working` field from the WorkSignal seam rather than from the stored row — the
// overlay is derived (inflight mutation OR agent chat mid-turn) and lives only in
// memory, so a home read that skipped it would report working=false while an
// agent chat anchored to the project home was mid-turn.
func TestGetHome_StampsWorkingFromSignal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-1").
		Return(domain.Workspace{ID: "ws-home-1", ProjectID: "proj-1", Kind: domain.WorkspaceKindHome}, nil)

	h := handlers.New(reader, nil, nil, nil, stubWork{working: true})
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

	h := handlers.New(reader, nil, nil, nil, stubWork{})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-1/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	reader.AssertExpectations(t)
}

// TestGetHome_Returns404WhenNotFound verifies that a GET /home for a project
// whose home workspace is missing returns HTTP 404. The handler lazily tries to
// provision a home workspace; if the project itself is not found (stubProjectReader
// returns ErrNotFound), the handler returns 404.
func TestGetHome_Returns404WhenNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-missing").
		Return(domain.Workspace{}, apperr.ErrNotFound)

	h := handlers.New(reader, stubProjectReader{}, nil, nil, stubWork{})
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

	h := handlers.New(reader, nil, nil, nil, stubWork{})
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

	h := handlers.New(reader, nil, &stubFiles{}, nil, stubWork{})
	r.GET("/projects/:projectId/home/files/tree", h.FileTree)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-2/home/files/tree", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	reader.AssertExpectations(t)
}

// ── resolveHome — lazy provisioning ───────────────────────────────────────

// TestGetHome_LazilyProvisions verifies that when GetHomeForProject reports
// ErrNotFound, resolveHome falls back to looking up the project and creating
// a home workspace from its path, supporting projects created before the
// home feature existed.
func TestGetHome_LazilyProvisions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-legacy").
		Return(domain.Workspace{}, apperr.ErrNotFound)
	newWS := domain.Workspace{
		ID:           "ws-new",
		ProjectID:    "proj-legacy",
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/legacy",
	}
	reader.On("CreateHome", mock.Anything, "proj-legacy", "/projects/legacy", mock.AnythingOfType("time.Time")).
		Return(newWS, nil)

	projects := &mockProjectReader{}
	projects.On("FindByKey", mock.Anything, "proj-legacy").
		Return(&domain.Project{ID: "proj-legacy", Path: "/projects/legacy"}, nil)

	h := handlers.New(reader, projects, nil, nil, stubWork{})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-legacy/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	reader.AssertExpectations(t)
	projects.AssertExpectations(t)
}

// TestGetHome_LazyProvisionCreateFails verifies that a CreateHome failure
// during lazy provisioning surfaces as a 500.
func TestGetHome_LazyProvisionCreateFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-legacy2").
		Return(domain.Workspace{}, apperr.ErrNotFound)
	reader.On("CreateHome", mock.Anything, "proj-legacy2", "/projects/legacy2", mock.AnythingOfType("time.Time")).
		Return(domain.Workspace{}, errors.New("write failed"))

	projects := &mockProjectReader{}
	projects.On("FindByKey", mock.Anything, "proj-legacy2").
		Return(&domain.Project{ID: "proj-legacy2", Path: "/projects/legacy2"}, nil)

	h := handlers.New(reader, projects, nil, nil, stubWork{})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-legacy2/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	reader.AssertExpectations(t)
	projects.AssertExpectations(t)
}

// TestGetHome_LazyProvisionProjectLookupErrors verifies that a non-nil error
// from the project lookup (distinct from a nil project) also yields 404.
func TestGetHome_LazyProvisionProjectLookupErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-err").
		Return(domain.Workspace{}, apperr.ErrNotFound)

	projects := &mockProjectReader{}
	projects.On("FindByKey", mock.Anything, "proj-err").
		Return(nil, errors.New("db unreachable"))

	h := handlers.New(reader, projects, nil, nil, stubWork{})
	r.GET("/projects/:projectId/home", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/projects/proj-err/home", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	reader.AssertExpectations(t)
	projects.AssertExpectations(t)
}

// ── RequireHomeWorkspace ───────────────────────────────────────────────────

// TestRequireHomeWorkspace_SetsWsIdAndCallsNext verifies the middleware
// injects the resolved home workspace id as the :wsId param and continues
// the chain to the downstream handler.
func TestRequireHomeWorkspace_SetsWsIdAndCallsNext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-mw").Return(domain.Workspace{
		ID:        "ws-mw",
		ProjectID: "proj-mw",
		Kind:      domain.WorkspaceKindHome,
	}, nil)

	h := handlers.New(reader, nil, nil, nil, stubWork{})
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

// TestRequireHomeWorkspace_AbortsChainOnFailure verifies that when the home
// workspace cannot be resolved, the middleware aborts and the downstream
// handler never runs.
func TestRequireHomeWorkspace_AbortsChainOnFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-abort").
		Return(domain.Workspace{}, errors.New("storage error"))

	h := handlers.New(reader, nil, nil, nil, stubWork{})
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
