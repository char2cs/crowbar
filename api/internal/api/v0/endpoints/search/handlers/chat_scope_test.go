package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/search/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

// This file pins the handler half of search's move onto /v0/chats/:chatId
// (spec §4.2's shared bucket, §8 step 4c): WHICH workspace a handler resolves.
//
// The old /v0/workspaces/:wsId/search... mount (and its WorkspaceReader seam)
// is gone as of spec §8 step 6 — resolveWorkspace now reads reqscope alone, so
// the trap this file guards is a chat-scoped request resolving to the empty
// workspace rather than the one resolveChatWorktree stashed.

// recordingEngine records the repoPath each Search/Replace call resolved to.
type recordingEngine struct {
	seenPaths []string
}

func (r *recordingEngine) Search(
	_ context.Context,
	repoPath string,
	_ enginesearch.SearchRequest,
) (enginesearch.SearchResponse, error) {
	r.seenPaths = append(r.seenPaths, repoPath)
	return enginesearch.SearchResponse{}, nil
}

func (r *recordingEngine) Replace(
	_ context.Context,
	repoPath string,
	_ enginesearch.ReplaceRequest,
	_ bool,
) error {
	r.seenPaths = append(r.seenPaths, repoPath)
	return nil
}

func searchRouterForScopes(
	t *testing.T,
	eng handlers.SearchEngine,
	resolved domain.Workspace,
) *gin.Engine {
	t.Helper()
	h := handlers.New(eng)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolved)
		c.Next()
	})
	chatScoped.POST("/search", h.Search)
	chatScoped.POST("/search/replace", h.Replace)

	return r
}

func doSearchRequestWithBody(
	t *testing.T,
	r *gin.Engine,
	path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// TestChatScoped_ResolvesFromReqscope is the core assertion: a chat-scoped
// request acts on the WORKSPACE resolveChatWorktree resolved, read back from
// reqscope, never the empty string a stale :wsId read would silently produce.
func TestChatScoped_ResolvesFromReqscope(t *testing.T) {
	eng := &recordingEngine{}
	resolved := domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"}
	r := searchRouterForScopes(t, eng, resolved)

	rec := doSearchRequestWithBody(t, r, "/v0/chats/chat-1/search", `{"query":"fmt"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, eng.seenPaths)
	assert.Equal(t, "/resolved", eng.seenPaths[len(eng.seenPaths)-1])
}

// TestWorkspaceScopedRouteIsGone proves spec §8 step 6's deletion is real: the
// old /v0/workspaces/:wsId/search mount this handler set used to also serve
// answers nothing on this router any more.
func TestWorkspaceScopedRouteIsGone(t *testing.T) {
	eng := &recordingEngine{}
	r := searchRouterForScopes(t, eng, domain.Workspace{ID: "ws-resolved"})

	rec := doSearchRequestWithBody(t, r, "/v0/workspaces/ws-direct/search", `{"query":"fmt"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestChatScopedReplace_ResolvesFromReqscope covers Replace separately from
// Search: it reads the same seam through a different handler method, which a
// partial re-key could miss.
func TestChatScopedReplace_ResolvesFromReqscope(t *testing.T) {
	eng := &recordingEngine{}
	resolved := domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"}
	r := searchRouterForScopes(t, eng, resolved)

	rec := doSearchRequestWithBody(t, r, "/v0/chats/chat-1/search/replace",
		`{"query":"fmt","replacement":"log"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, eng.seenPaths)
	assert.Equal(t, "/resolved", eng.seenPaths[len(eng.seenPaths)-1])
}
