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

func (stubFiles) ReadContent(_ context.Context, _, _ string) (domain.FileContent, error) {
	return domain.FileContent{Content: "hello"}, nil
}

func (stubFiles) WriteContent(_ context.Context, _, _, _, _ string, _ time.Time) error { return nil }

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
	rg.GET("/workspaces/:wsId/files/content", h.ReadContent)
	rg.PUT("/workspaces/:wsId/files/content", h.SaveContent)
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

	rec = do(r, http.MethodGet, "/v0/workspaces/ws1/files/content?path=a.go", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPut, "/v0/workspaces/ws1/files/content",
		map[string]any{"path": "a.go", "content": "hi"})
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

func TestFileHandlers_ReadContent_MissingPath(
	t *testing.T,
) {
	r := newRouter(stubFiles{})
	rec := do(r, http.MethodGet, "/v0/workspaces/ws1/files/content", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
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

// trackingFiles records which of CreateDir/CreateFile Create actually routed
// to, proving the "dir"/"directory" type switch dispatches to CreateDir while
// every other type (including the default "file") goes through CreateFile —
// not just that both return 201.
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

// TestFileHandlers_Create_MissingPath proves Create's own guard rejects an
// empty path with 400 before ever dispatching to CreateFile/CreateDir.
func TestFileHandlers_Create_MissingPath(
	t *testing.T,
) {
	r := newRouter(stubFiles{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/files",
		map[string]any{"type": "file"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestFileHandlers_SaveContent_MissingPath proves SaveContent's own guard
// rejects an empty path with 400 before calling WriteContent.
func TestFileHandlers_SaveContent_MissingPath(
	t *testing.T,
) {
	r := newRouter(stubFiles{})

	rec := do(r, http.MethodPut, "/v0/workspaces/ws1/files/content",
		map[string]any{"content": "hi"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestFileHandlers_Rename_MissingFields proves Rename's own guard rejects a
// request missing either path or newPath with 400 before calling Rename.
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

// nilTreeFiles reports success but no nodes, distinguishing "an empty
// directory" (nodes == nil, which Tree must normalise to an empty array for
// the wire) from an actual usecase error.
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

// doRaw posts a raw, possibly-malformed body — unlike do (which
// json.Marshals a Go value, so it can never produce actually-invalid JSON),
// this is how each handler's ShouldBindJSON decode-failure branch gets
// exercised.
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

func TestFileHandlers_SaveContent_BadJSON_Returns400(
	t *testing.T,
) {
	r := newRouter(stubFiles{})
	rec := doRaw(r, http.MethodPut, "/v0/workspaces/ws1/files/content", `{"path":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
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
