package identity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/identity"
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

// registerChatScoped wires identity.Register the way router.go does: on the
// flat chat-scoped group alone (spec §8 step 6 retired the old
// workspace-scoped mount).
func registerChatScoped(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	identity.Register(v0.Group("/chats/:chatId"), stubResolver{})
	return r
}

// TestRegisterMountsChatScopedRoute is the route half of this step: the
// identity route is reachable at the flat /v0/chats/:chatId prefix (spec
// §7.1).
func TestRegisterMountsChatScopedRoute(
	t *testing.T,
) {
	r := registerChatScoped(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat1/identity", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}

// TestRegisterDropsWorkspaceScopedRoute proves spec §8 step 6's deletion is
// real for identity: the old /v0/workspaces/:wsId/identity mount, kept alive
// alongside the chat-scoped one through the rest of this refactor, answers
// nothing any more.
func TestRegisterDropsWorkspaceScopedRoute(
	t *testing.T,
) {
	r := registerChatScoped(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/identity", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
