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

func (stubFiles) ReadContent(
	_ context.Context,
	_ string,
	_ string,
) (domain.FileContent, error) {
	return domain.FileContent{}, nil
}

func (stubFiles) WriteContent(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) error {
	return nil
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

// filesSurface is the method+relative-path set files.Register mounts, written
// once and asserted against BOTH live prefixes. The relative half is
// deliberately prefix-free: a route that reached only one of the two mounts is
// the failure this shape makes impossible to miss.
//
// /ws is in the list like any other route. It is its own leaf here rather than
// a dual-serve of /tree the way git's status route is, so nothing distinguishes
// it at the routing level — and a mount that dropped it would take the live
// file-change stream with it.
func filesSurface() []struct {
	method string
	path   string
} {
	return []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/tree"},
		{http.MethodGet, "/content"},
		{http.MethodPut, "/content"},
		{http.MethodPost, ""},
		{http.MethodPost, "/copy"},
		{http.MethodPatch, ""},
		{http.MethodDelete, ""},
		{http.MethodGet, "/ws"},
	}
}

// registerBothMounts wires files.Register the way router.go does: the old
// workspace-scoped group and the flat chat-scoped one, on one engine.
func registerBothMounts(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	files.Register(v0, v0.Group("/chats/:chatId"), stubFiles{}, func(_ *gin.Context) {})
	return r
}

// TestRegisterMountsChatScopedRoutes is the route half of this step: every
// files route is reachable at the flat /v0/chats/:chatId prefix (spec §7.1).
func TestRegisterMountsChatScopedRoutes(
	t *testing.T,
) {
	r := registerBothMounts(t)

	for _, tc := range filesSurface() {
		path := "/v0/chats/chat1/files" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegisterKeepsWorkspaceScopedRoutes is the regression bar for the
// coexistence this step deliberately ships: the workspace-scoped surface is NOT
// retired here (spec §8 step 6 does that, once every group has moved), so every
// one of its routes must still answer exactly as before.
func TestRegisterKeepsWorkspaceScopedRoutes(
	t *testing.T,
) {
	r := registerBothMounts(t)

	for _, tc := range filesSurface() {
		path := "/v0/workspaces/ws1/files" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}
