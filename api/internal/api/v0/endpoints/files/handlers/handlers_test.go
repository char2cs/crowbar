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
func (stubFiles) WriteContent(_ context.Context, _, _, _ string, _ time.Time) error { return nil }
func (stubFiles) CreateFile(_ context.Context, _, _ string, _ time.Time) error      { return nil }
func (stubFiles) CreateDir(_ context.Context, _, _ string, _ time.Time) error       { return nil }
func (stubFiles) Rename(_ context.Context, _, _, _ string, _ time.Time) error       { return nil }
func (stubFiles) Delete(_ context.Context, _, _ string, _ time.Time) error          { return nil }

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
