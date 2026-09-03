package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubResolver struct{}

func (stubResolver) CurrentIdentity(
	_ context.Context,
	_ string,
) gitdomain.Identity {
	return gitdomain.Identity{}
}

type stubReader struct{}

func (stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

// registerBothMounts wires identity.Register the way router.go does: the old
// workspace-scoped group and the flat chat-scoped one, on one engine.
func registerBothMounts(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	identity.Register(v0, v0.Group("/chats/:chatId"), stubResolver{}, stubReader{})
	return r
}

// TestRegisterMountsChatScopedRoute is the route half of this step: the
// identity route is reachable at the flat /v0/chats/:chatId prefix (spec
// §7.1).
func TestRegisterMountsChatScopedRoute(
	t *testing.T,
) {
	r := registerBothMounts(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat1/identity", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}

// TestRegisterKeepsWorkspaceScopedRoute is the regression bar for the
// coexistence this step deliberately ships: the workspace-scoped surface is
// NOT retired here (spec §8 step 6 does that, once every group has moved), so
// it must still answer exactly as before.
func TestRegisterKeepsWorkspaceScopedRoute(
	t *testing.T,
) {
	r := registerBothMounts(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/identity", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}
