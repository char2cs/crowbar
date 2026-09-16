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

// This file pins the handler half of provider's move onto /v0/chats/:chatId
// (spec §4.2's OWNED bucket, §8 step 5): WHICH workspace a State poll
// resolves.
//
// Unlike editor/LSP's session key, PollOnView's own wsID parameter is never
// consulted by the production engine (it polls purely from repoPath/branch),
// so there is no owner-id seam to pin here the way editor's is — only which
// worktree the poll actually runs against.

// recordingEngine embeds nothing and records the worktree path/branch
// PollOnView was called with.
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

// resolvedWorkspace is the workspace the chat group's resolveChatWorktree
// middleware stashes for chat "chat-1". Its worktree path and branch
// deliberately differ from any other test fixture in this package, so a
// handler that reached for the wrong source would fail rather than pass by
// coincidence.
func resolvedWorkspace() domain.Workspace {
	return domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved/path", Branch: "resolved-branch"}
}

// providerRouterForScopes wires provider's live State mount the way
// router.go does, including the chat group's middleware.
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

// TestChatScopedState_PollsTheResolvedWorktree proves a provider poll reached
// through a chat runs against the WORKSPACE that chat resolves to, not the
// chat id and not an empty path.
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

// TestWorkspaceScopedRouteIsGone proves spec §8 step 6's deletion is real:
// the old /v0/workspaces/:wsId/provider mount this handler set used to also
// serve answers nothing on this router any more.
func TestWorkspaceScopedRouteIsGone(t *testing.T) {
	eng := &recordingEngine{}
	r := providerRouterForScopes(t, eng, stubReader{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/provider", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
