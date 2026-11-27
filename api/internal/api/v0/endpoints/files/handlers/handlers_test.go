package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubFiles struct{}

func (stubFiles) Tree(_ context.Context, _, _ string, _ file.FileStatusProvider) ([]domain.FileNode, error) {
	return []domain.FileNode{{Path: "a.go"}}, nil
}

func (stubFiles) CreateFile(_ context.Context, _, _ string, _ time.Time) error { return nil }

func (stubFiles) CreateDir(_ context.Context, _, _ string, _ time.Time) error { return nil }

func (stubFiles) Copy(_ context.Context, _, _, _ string, _ time.Time) error { return nil }

func (stubFiles) Rename(_ context.Context, _, _, _ string, _ time.Time) error { return nil }

func (stubFiles) Delete(_ context.Context, _, _ string, _ time.Time) error { return nil }

func newRouter(
	f handlers.Files,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(f)
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/files/tree", h.Tree)
	rg.POST("/workspaces/:wsId/files", h.Create)
	rg.POST("/workspaces/:wsId/files/copy", h.Copy)
	rg.PATCH("/workspaces/:wsId/files", h.Rename)
	rg.DELETE("/workspaces/:wsId/files", h.Delete)
	return r
}

func do(
	r *gin.Engine,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
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

func TestFileHandlers_HappyPath(
	t *testing.T,
) {
	r := newRouter(stubFiles{})

	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/files/tree", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/workspaces/ws1/files",
		map[string]any{"path": "new.go", "type": "file"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodPost, "/v0/workspaces/ws1/files/copy",
		map[string]any{"sourcePath": "a.go", "destPath": "a copy.go"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodPatch, "/v0/workspaces/ws1/files",
		map[string]any{"path": "a.go", "newPath": "b.go"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodDelete, "/v0/workspaces/ws1/files",
		map[string]any{"path": "b.go"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodDelete, "/v0/workspaces/ws1/files?path=b.go", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestFileHandlers_Copy_MissingPaths(
	t *testing.T,
) {
	r := newRouter(stubFiles{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/files/copy",
		map[string]any{"destPath": "a copy.go"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = do(r, http.MethodPost, "/v0/workspaces/ws1/files/copy",
		map[string]any{"sourcePath": "a.go"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

type trackingFiles struct {
	stubFiles
	dirCalls  []string
	fileCalls []string
}

func (t *trackingFiles) CreateDir(
	_ context.Context,
	_, path string,
	_ time.Time,
) error {
	t.dirCalls = append(t.dirCalls, path)
	return nil
}

func (t *trackingFiles) CreateFile(
	_ context.Context,
	_, path string,
	_ time.Time,
) error {
	t.fileCalls = append(t.fileCalls, path)
	return nil
}

func TestFileHandlers_Create_RoutesByType(
	t *testing.T,
) {
	tf := &trackingFiles{}
	r := newRouter(tf)

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/files",
		map[string]any{"path": "newdir", "type": "dir"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodPost, "/v0/workspaces/ws1/files",
		map[string]any{"path": "newdir2", "type": "directory"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodPost, "/v0/workspaces/ws1/files",
		map[string]any{"path": "newfile", "type": "file"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	assert.Equal(t, []string{"newdir", "newdir2"}, tf.dirCalls,
		"both \"dir\" and \"directory\" must route to CreateDir")
	assert.Equal(t, []string{"newfile"}, tf.fileCalls,
		"a \"file\" type must route to CreateFile, not CreateDir")
}

func TestFileHandlers_Create_MissingPath(
	t *testing.T,
) {
	r := newRouter(stubFiles{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/files",
		map[string]any{"type": "file"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFileHandlers_Rename_MissingFields(
	t *testing.T,
) {
	r := newRouter(stubFiles{})

	rec := do(r, http.MethodPatch, "/v0/workspaces/ws1/files",
		map[string]any{"newPath": "b.go"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = do(r, http.MethodPatch, "/v0/workspaces/ws1/files",
		map[string]any{"path": "a.go"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFileHandlers_Delete_NoPathAnywhere_Returns400(
	t *testing.T,
) {
	r := newRouter(stubFiles{})

	rec := do(r, http.MethodDelete, "/v0/workspaces/ws1/files", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

type nilTreeFiles struct{ stubFiles }

func (nilTreeFiles) Tree(
	_ context.Context, _, _ string, _ file.FileStatusProvider,
) ([]domain.FileNode, error) {
	return nil, nil
}

func TestFileHandlers_Tree_NilNodes_ReturnsEmptyArray(
	t *testing.T,
) {
	r := newRouter(nilTreeFiles{})

	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/files/tree", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data []domain.FileNode `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.NotNil(t, env.Data, "a nil node list must be normalised to [] on the wire, not null")
	assert.Empty(t, env.Data)
}

func doRaw(
	r *gin.Engine,
	method, path, rawBody string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(rawBody)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestFileHandlers_Create_BadJSON_Returns400(
	t *testing.T,
) {
	r := newRouter(stubFiles{})
	rec := doRaw(r, http.MethodPost, "/v0/workspaces/ws1/files", `{"path":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFileHandlers_Copy_BadJSON_Returns400(
	t *testing.T,
) {
	r := newRouter(stubFiles{})
	rec := doRaw(r, http.MethodPost, "/v0/workspaces/ws1/files/copy", `{"sourcePath":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFileHandlers_Rename_BadJSON_Returns400(
	t *testing.T,
) {
	r := newRouter(stubFiles{})
	rec := doRaw(r, http.MethodPatch, "/v0/workspaces/ws1/files", `{"path":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
