package handlers_test

import (
	"bytes"
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

// ── mockFiles — a controllable Files double for exercising success/error
// branches in the home file handlers. ─────────────────────────────────────

type mockFiles struct{ mock.Mock }

func (m *mockFiles) Tree(
	ctx context.Context, wsID, dirPath string, provider fileusecase.FileStatusProvider,
) ([]domain.FileNode, error) {
	args := m.Called(ctx, wsID, dirPath, provider)
	nodes, _ := args.Get(0).([]domain.FileNode)
	return nodes, args.Error(1)
}

func (m *mockFiles) ReadContent(
	ctx context.Context, wsID, filePath string,
) (domain.FileContent, error) {
	args := m.Called(ctx, wsID, filePath)
	content, _ := args.Get(0).(domain.FileContent)
	return content, args.Error(1)
}

func (m *mockFiles) WriteContent(
	ctx context.Context, wsID, filePath, content, encoding string, now time.Time,
) error {
	args := m.Called(ctx, wsID, filePath, content, encoding, now)
	return args.Error(0)
}

func (m *mockFiles) CreateFile(
	ctx context.Context, wsID, filePath string, now time.Time,
) error {
	args := m.Called(ctx, wsID, filePath, now)
	return args.Error(0)
}

func (m *mockFiles) CreateDir(
	ctx context.Context, wsID, dirPath string, now time.Time,
) error {
	args := m.Called(ctx, wsID, dirPath, now)
	return args.Error(0)
}

func (m *mockFiles) Copy(
	ctx context.Context, wsID, sourcePath, destPath string, now time.Time,
) error {
	args := m.Called(ctx, wsID, sourcePath, destPath, now)
	return args.Error(0)
}

func (m *mockFiles) Rename(
	ctx context.Context, wsID, oldPath, newPath string, now time.Time,
) error {
	args := m.Called(ctx, wsID, oldPath, newPath, now)
	return args.Error(0)
}

func (m *mockFiles) Delete(
	ctx context.Context, wsID, filePath string, now time.Time,
) error {
	args := m.Called(ctx, wsID, filePath, now)
	return args.Error(0)
}

// ── shared fixtures ───────────────────────────────────────────────────────

func homeWorkspaceReader(t *testing.T, projectID, wsID string) *mockHomeReader {
	t.Helper()
	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, projectID).Return(domain.Workspace{
		ID:           wsID,
		ProjectID:    projectID,
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/" + projectID,
	}, nil)
	return reader
}

func doReq(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// ── FileTree ────────────────────────────────────────────────────────────

func TestFileTree_ErrorFromUsecase_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Tree", mock.Anything, "ws-1", ".", mock.Anything).
		Return(nil, errors.New("boom"))

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.GET("/projects/:projectId/home/files/tree", h.FileTree)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/files/tree", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	files.AssertExpectations(t)
}

func TestFileTree_NilNodes_ReturnsEmptyArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Tree", mock.Anything, "ws-1", "sub", mock.Anything).
		Return(nil, nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.GET("/projects/:projectId/home/files/tree", h.FileTree)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/files/tree?path=sub", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data []domain.FileNode `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.NotNil(t, env.Data)
	require.Empty(t, env.Data)
}

// ── FileContent ─────────────────────────────────────────────────────────

func TestFileContent_Returns200WithContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("ReadContent", mock.Anything, "ws-1", "a.txt").
		Return(domain.FileContent{Content: "hello"}, nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.GET("/projects/:projectId/home/files/content", h.FileContent)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/files/content?path=a.txt", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data domain.FileContent `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, "hello", env.Data.Content)
	files.AssertExpectations(t)
}

func TestFileContent_MissingPath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.GET("/projects/:projectId/home/files/content", h.FileContent)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/files/content", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFileContent_NotFound_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("ReadContent", mock.Anything, "ws-1", "missing.txt").
		Return(domain.FileContent{}, apperr.ErrNotFound)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.GET("/projects/:projectId/home/files/content", h.FileContent)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/files/content?path=missing.txt", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestFileContent_WorkspaceResolutionFails_NoUsecaseCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-x").
		Return(domain.Workspace{}, errors.New("storage down"))
	files := &mockFiles{}

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.GET("/projects/:projectId/home/files/content", h.FileContent)

	rec := doReq(r, http.MethodGet, "/projects/proj-x/home/files/content?path=a.txt", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	files.AssertNotCalled(t, "ReadContent", mock.Anything, mock.Anything, mock.Anything)
}

// ── SaveFileContent ─────────────────────────────────────────────────────

func TestSaveFileContent_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("WriteContent", mock.Anything, "ws-1", "a.txt", "new body", "", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.PUT("/projects/:projectId/home/files/content", h.SaveFileContent)

	rec := doReq(r, http.MethodPut, "/projects/proj-1/home/files/content", map[string]any{
		"path": "a.txt", "content": "new body",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	files.AssertExpectations(t)
}

func TestSaveFileContent_BadJSON_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.PUT("/projects/:projectId/home/files/content", h.SaveFileContent)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/projects/proj-1/home/files/content", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSaveFileContent_MissingPath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.PUT("/projects/:projectId/home/files/content", h.SaveFileContent)

	rec := doReq(r, http.MethodPut, "/projects/proj-1/home/files/content", map[string]any{"content": "x"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSaveFileContent_UsecaseError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("WriteContent", mock.Anything, "ws-1", "a.txt", "x", "", mock.AnythingOfType("time.Time")).
		Return(errors.New("disk full"))

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.PUT("/projects/:projectId/home/files/content", h.SaveFileContent)

	rec := doReq(r, http.MethodPut, "/projects/proj-1/home/files/content", map[string]any{
		"path": "a.txt", "content": "x",
	})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ── CreateFile ──────────────────────────────────────────────────────────

func TestCreateFile_File_Returns201(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("CreateFile", mock.Anything, "ws-1", "new.txt", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.POST("/projects/:projectId/home/files", h.CreateFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files", map[string]any{"path": "new.txt"})
	require.Equal(t, http.StatusCreated, rec.Code)
	files.AssertExpectations(t)
}

func TestCreateFile_Directory_Returns201(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("CreateDir", mock.Anything, "ws-1", "newdir", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.POST("/projects/:projectId/home/files", h.CreateFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files", map[string]any{"path": "newdir", "type": "dir"})
	require.Equal(t, http.StatusCreated, rec.Code)
	files.AssertExpectations(t)
}

func TestCreateFile_DirectoryTypeAlias_Returns201(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("CreateDir", mock.Anything, "ws-1", "newdir2", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.POST("/projects/:projectId/home/files", h.CreateFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files", map[string]any{"path": "newdir2", "type": "directory"})
	require.Equal(t, http.StatusCreated, rec.Code)
	files.AssertExpectations(t)
}

func TestCreateFile_DirectoryUsecaseError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("CreateDir", mock.Anything, "ws-1", "faildir", mock.AnythingOfType("time.Time")).
		Return(errors.New("mkdir failed"))

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.POST("/projects/:projectId/home/files", h.CreateFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files", map[string]any{"path": "faildir", "type": "dir"})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	files.AssertExpectations(t)
}

func TestCreateFile_MissingPath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.POST("/projects/:projectId/home/files", h.CreateFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files", map[string]any{})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateFile_AlreadyExists_ReturnsConflictMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("CreateFile", mock.Anything, "ws-1", "exists.txt", mock.AnythingOfType("time.Time")).
		Return(errors.New("already exists"))

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.POST("/projects/:projectId/home/files", h.CreateFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files", map[string]any{"path": "exists.txt"})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ── CopyFile ────────────────────────────────────────────────────────────

func TestCopyFile_Returns201(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Copy", mock.Anything, "ws-1", "a.txt", "a copy.txt", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.POST("/projects/:projectId/home/files/copy", h.CopyFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files/copy", map[string]any{
		"sourcePath": "a.txt", "destPath": "a copy.txt",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	files.AssertExpectations(t)
}

func TestCopyFile_MissingFields_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.POST("/projects/:projectId/home/files/copy", h.CopyFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files/copy", map[string]any{"sourcePath": "a.txt"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCopyFile_BadJSON_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.POST("/projects/:projectId/home/files/copy", h.CopyFile)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/projects/proj-1/home/files/copy", bytes.NewReader([]byte("{bad")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCopyFile_UsecaseError_Returns404WhenNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Copy", mock.Anything, "ws-1", "ghost.txt", "ghost copy.txt", mock.AnythingOfType("time.Time")).
		Return(errors.New("no such file or directory"))

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.POST("/projects/:projectId/home/files/copy", h.CopyFile)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/files/copy", map[string]any{
		"sourcePath": "ghost.txt", "destPath": "ghost copy.txt",
	})
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ── RenameFile ──────────────────────────────────────────────────────────

func TestRenameFile_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Rename", mock.Anything, "ws-1", "old.txt", "new.txt", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.PATCH("/projects/:projectId/home/files", h.RenameFile)

	rec := doReq(r, http.MethodPatch, "/projects/proj-1/home/files", map[string]any{
		"path": "old.txt", "newPath": "new.txt",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	files.AssertExpectations(t)
}

// TestRegression_RenameFile_AcceptsPathNewPathContract pins the home rename
// handler to the shared {path, newPath} shape the workspace handler and the FE
// already use. It previously bound {oldPath, newPath}, so every home-project
// explorer rename 400'd on the mismatch. Red against the old binding: {path,
// newPath} left `path` empty → the required-fields 400.
func TestRegression_RenameFile_AcceptsPathNewPathContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Rename", mock.Anything, "ws-1", "a.txt", "b.txt", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.PATCH("/projects/:projectId/home/files", h.RenameFile)

	rec := doReq(r, http.MethodPatch, "/projects/proj-1/home/files", map[string]any{
		"path": "a.txt", "newPath": "b.txt",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	// The rename must reach the usecase with the bound path as the old path.
	files.AssertCalled(t, "Rename", mock.Anything, "ws-1", "a.txt", "b.txt", mock.AnythingOfType("time.Time"))

	// The retired {oldPath, newPath} shape now leaves `path` unbound → 400.
	rec2 := doReq(r, http.MethodPatch, "/projects/proj-1/home/files", map[string]any{
		"oldPath": "a.txt", "newPath": "b.txt",
	})
	require.Equal(t, http.StatusBadRequest, rec2.Code)
}

func TestRenameFile_MissingFields_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.PATCH("/projects/:projectId/home/files", h.RenameFile)

	rec := doReq(r, http.MethodPatch, "/projects/proj-1/home/files", map[string]any{"path": "old.txt"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameFile_BadJSON_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.PATCH("/projects/:projectId/home/files", h.RenameFile)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/projects/proj-1/home/files", bytes.NewReader([]byte("{bad")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRenameFile_UsecaseError_Returns404WhenNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Rename", mock.Anything, "ws-1", "ghost.txt", "new.txt", mock.AnythingOfType("time.Time")).
		Return(errors.New("no such file or directory"))

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.PATCH("/projects/:projectId/home/files", h.RenameFile)

	rec := doReq(r, http.MethodPatch, "/projects/proj-1/home/files", map[string]any{
		"path": "ghost.txt", "newPath": "new.txt",
	})
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ── DeleteFile ──────────────────────────────────────────────────────────

func TestDeleteFile_JSONBody_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Delete", mock.Anything, "ws-1", "a.txt", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.DELETE("/projects/:projectId/home/files", h.DeleteFile)

	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/files", map[string]any{"path": "a.txt"})
	require.Equal(t, http.StatusOK, rec.Code)
	files.AssertExpectations(t)
}

func TestDeleteFile_QueryParamFallback_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Delete", mock.Anything, "ws-1", "b.txt", mock.AnythingOfType("time.Time")).
		Return(nil)

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.DELETE("/projects/:projectId/home/files", h.DeleteFile)

	// No body at all — DELETE with an empty request falls through ShouldBindJSON's
	// EOF error, so the handler must recover the path from the query string.
	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/files?path=b.txt", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	files.AssertExpectations(t)
}

func TestDeleteFile_MissingPath_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	h := handlers.New(reader, nil, &mockFiles{}, nil, stubWork{})
	r.DELETE("/projects/:projectId/home/files", h.DeleteFile)

	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/files", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteFile_UsecaseError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeWorkspaceReader(t, "proj-1", "ws-1")
	files := &mockFiles{}
	files.On("Delete", mock.Anything, "ws-1", "a.txt", mock.AnythingOfType("time.Time")).
		Return(errors.New("permission denied"))

	h := handlers.New(reader, nil, files, nil, stubWork{})
	r.DELETE("/projects/:projectId/home/files", h.DeleteFile)

	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/files", map[string]any{"path": "a.txt"})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
