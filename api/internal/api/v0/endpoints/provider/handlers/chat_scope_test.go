package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/provider/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

type recordingEngine struct {
	seenPath   string
	seenBranch string
}

func (e *recordingEngine) PollOnView(
	_ context.Context,
	_ string,
	repoPath string,
	branch string,
) (engineprovider.ProviderState, error) {
	e.seenPath = repoPath
	e.seenBranch = branch
	return engineprovider.ProviderState{}, nil
}

func (e *recordingEngine) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return nil, nil
}

var _ handlers.ProviderEngine = (*recordingEngine)(nil)

func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved/path", Branch: "resolved-branch", Provisioning: domain.WorkspaceProvisioned}
}

func providerRouterForScopes(
	t *testing.T,
	eng handlers.ProviderEngine,
	wsReader handlers.WorkspaceReader,
) *gin.Engine {
	t.Helper()
	h := handlers.New(eng, wsReader)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolvedWorkspace())
		c.Next()
	})
	chatScoped.GET("/provider", h.State)

	return r
}

func TestChatScopedState_PollsTheResolvedWorktree(t *testing.T) {
	eng := &recordingEngine{}
	r := providerRouterForScopes(t, eng, stubReader{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat-1/provider", http.NoBody)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/resolved/path", eng.seenPath)
	assert.Equal(t, "resolved-branch", eng.seenBranch)
}

func TestWorkspaceScopedRouteIsGone(t *testing.T) {
	eng := &recordingEngine{}
	r := providerRouterForScopes(t, eng, stubReader{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/provider", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
