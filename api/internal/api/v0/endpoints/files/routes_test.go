package files_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubFiles struct{}

func (stubFiles) Tree(
	_ context.Context,
	_ string,
	_ string,
	_ fileusecase.FileStatusProvider,
) ([]domain.FileNode, error) {
	return nil, nil
}

func (stubFiles) CreateFile(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) CreateDir(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) Copy(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) Rename(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func (stubFiles) Delete(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
}

func filesSurface() []struct {
	method string
	path   string
} {
	return []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/tree"},
		{http.MethodPost, ""},
		{http.MethodPost, "/copy"},
		{http.MethodPatch, ""},
		{http.MethodDelete, ""},
	}
}

func registerChatScoped(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	files.Register(v0.Group("/chats/:chatId"), stubFiles{})
	return r
}

func TestRegisterMountsChatScopedRoutes(
	t *testing.T,
) {
	r := registerChatScoped(t)

	for _, tc := range filesSurface() {
		path := "/v0/chats/chat1/files" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

func TestRegisterDropsWorkspaceScopedRoutes(
	t *testing.T,
) {
	r := registerChatScoped(t)

	for _, tc := range filesSurface() {
		path := "/v0/workspaces/ws1/files" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}
