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

func registerChatScoped(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	identity.Register(v0.Group("/chats/:chatId"), stubResolver{})
	return r
}

func TestRegisterMountsChatScopedRoute(
	t *testing.T,
) {
	r := registerChatScoped(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat1/identity", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestRegisterDropsWorkspaceScopedRoute(
	t *testing.T,
) {
	r := registerChatScoped(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/identity", http.NoBody)
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
