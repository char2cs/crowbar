package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type errFiles struct{ err error }

func (e errFiles) Tree(_ context.Context, _, _ string, _ file.FileStatusProvider) ([]domain.FileNode, error) {
	return nil, e.err
}
func (e errFiles) ReadContent(_ context.Context, _, _ string) (domain.FileContent, error) {
	return domain.FileContent{}, e.err
}
func (e errFiles) WriteContent(_ context.Context, _, _, _ string, _ time.Time) error { return e.err }
func (e errFiles) CreateFile(_ context.Context, _, _ string, _ time.Time) error      { return e.err }
func (e errFiles) CreateDir(_ context.Context, _, _ string, _ time.Time) error       { return e.err }
func (e errFiles) Rename(_ context.Context, _, _, _ string, _ time.Time) error       { return e.err }
func (e errFiles) Delete(_ context.Context, _, _ string, _ time.Time) error          { return e.err }

func TestFileHandlers_ErrorPaths(
	t *testing.T,
) {
	r := newRouter(errFiles{err: errors.New("boom")})

	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, "/v0/workspaces/ws1/files", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, "/v0/workspaces/ws1/files/content?path=a.go", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPut, "/v0/workspaces/ws1/files/content",
		map[string]any{"path": "a.go", "content": "hi"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, "/v0/workspaces/ws1/files",
		map[string]any{"path": "new.go", "type": "file"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPatch, "/v0/workspaces/ws1/files",
		map[string]any{"from": "a.go", "to": "b.go"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodDelete, "/v0/workspaces/ws1/files?path=a.go", nil).Code)
}

func TestFileHandlers_NotFoundError(
	t *testing.T,
) {
	r := newRouter(errFiles{err: errors.New("file not found")})

	assert.Equal(t, http.StatusNotFound, do(r, http.MethodGet, "/v0/workspaces/ws1/files/content?path=missing.go", nil).Code)
}

