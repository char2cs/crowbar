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
// (spec §4.2's shared bucket, §8 step 4c): WHICH workspace a handler resolves,
// for each of the two prefixes the surface is currently mounted at.
//
// Unlike review, search DOES have a WorkspaceReader seam — the same shape
// terminal's original one had. The trap here is not an empty id (wsReader.Get
// would fail loudly on that): it is resolving the SAME workspace a second
// time from a URL param that no longer exists on the chat-scoped route,
// silently returning "workspace not found" for every chat-scoped request.

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

// failingReader always errors. It stands in for wsReader on the chat-scoped
// mount to prove resolveWorkspace never falls through to it once reqscope has
// already answered.
type failingReader struct{}

func (failingReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, assert.AnError
}

// directReader resolves wsId path params to a fixed workspace, standing in
// for the old workspace-scoped mount's real repository read.
type directReader struct {
	ws domain.Workspace
}

func (d directReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return d.ws, nil
}

func searchRouterForScopes(
	t *testing.T,
	eng handlers.SearchEngine,
	wsReader handlers.WorkspaceReader,
	resolved domain.Workspace,
) *gin.Engine {
	t.Helper()
	h := handlers.New(eng, wsReader)
	r := gin.New()

	chatScoped := r.Group("/v0/chats/:chatId")
	chatScoped.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, resolved)
		c.Next()
	})
	chatScoped.POST("/search", h.Search)
	chatScoped.POST("/search/replace", h.Replace)

	wsScoped := r.Group("/v0")
	wsScoped.POST("/workspaces/:wsId/search", h.Search)
	wsScoped.POST("/workspaces/:wsId/search/replace", h.Replace)

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

// TestChatScoped_ResolvesFromReqscopeNeverTheReader is the core assertion: a
// chat-scoped request never falls through to the (here, always-failing)
// wsReader — resolveWorkspace must have taken the reqscope branch.
func TestChatScoped_ResolvesFromReqscopeNeverTheReader(t *testing.T) {
	eng := &recordingEngine{}
	resolved := domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"}
	r := searchRouterForScopes(t, eng, failingReader{}, resolved)

	rec := doSearchRequestWithBody(t, r, "/v0/chats/chat-1/search", `{"query":"fmt"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, eng.seenPaths)
	assert.Equal(t, "/resolved", eng.seenPaths[len(eng.seenPaths)-1])
}

// TestWorkspaceScopedRoutesStillUseTheReader is the regression bar for the
// mount this step deliberately leaves standing: the old route keeps resolving
// its workspace from the reader keyed by :wsId, and reqscope — never set on
// that group — must not have displaced it.
func TestWorkspaceScopedRoutesStillUseTheReader(t *testing.T) {
	eng := &recordingEngine{}
	reader := directReader{ws: domain.Workspace{ID: "ws-direct", WorktreePath: "/direct"}}
	r := searchRouterForScopes(t, eng, reader, domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"})

	rec := doSearchRequestWithBody(t, r, "/v0/workspaces/ws-direct/search", `{"query":"fmt"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, eng.seenPaths)
	assert.Equal(t, "/direct", eng.seenPaths[len(eng.seenPaths)-1])
}

// TestChatScopedReplace_ResolvesFromReqscope covers Replace separately from
// Search: it reads the same seam through a different handler method, which a
// partial re-key could miss.
func TestChatScopedReplace_ResolvesFromReqscope(t *testing.T) {
	eng := &recordingEngine{}
	resolved := domain.Workspace{ID: "ws-resolved", WorktreePath: "/resolved"}
	r := searchRouterForScopes(t, eng, failingReader{}, resolved)

	rec := doSearchRequestWithBody(t, r, "/v0/chats/chat-1/search/replace",
		`{"query":"fmt","replacement":"log"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotEmpty(t, eng.seenPaths)
	assert.Equal(t, "/resolved", eng.seenPaths[len(eng.seenPaths)-1])
}
