package v0

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/usecases/pairing"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeRepoReader struct {
	repo   *domain.Repository
	err    error
	called int
}

func (f *fakeRepoReader) FindByKey(_ context.Context, _ string) (*domain.Repository, error) {
	f.called++
	return f.repo, f.err
}

func mountRepoGuard(reader repoScopeReader) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/v0/projects/:projectId")
	grp.Use(scopeRepoToPath(reader))
	grp.GET("/repos/:repoId/thing", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	grp.DELETE("/repos/:repoId", func(c *gin.Context) { c.String(http.StatusOK, "deleted") })
	grp.GET("/repos", func(c *gin.Context) { c.String(http.StatusOK, "list") })
	return r
}

// TestRegression_RepoScopeGuard_RejectsCrossProjectRepoId proves pass-4: a :repoId
// from a different project must 404, not let a destructive op (DeleteRepo, icon
// write) touch another project's repo.
func TestRegression_RepoScopeGuard_RejectsCrossProjectRepoId(t *testing.T) {
	reader := &fakeRepoReader{repo: &domain.Repository{ID: "R", ProjectID: "pX"}}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/v0/projects/pA/repos/R", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"a :repoId from a different project must 404 before the handler")
}

// TestRegression_RepoScopeGuard_AllowsMatchingProject proves an in-scope repo
// reaches the handler.
func TestRegression_RepoScopeGuard_AllowsMatchingProject(t *testing.T) {
	reader := &fakeRepoReader{repo: &domain.Repository{ID: "R", ProjectID: "pA"}}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/R/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRegression_RepoScopeGuard_404OnMissingRepo proves a non-existent repo 404s.
func TestRegression_RepoScopeGuard_404OnMissingRepo(t *testing.T) {
	reader := &fakeRepoReader{repo: nil}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/ghost/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestRegression_RepoScopeGuard_ReturnsMappedErrorOnReaderFailure proves a
// repository-lookup error (a storage read failure, not a missing repo) is
// mapped through libs.StatusAndMessage and aborts the chain, rather than
// falling through to the generic 404 a nil repo gets or reaching the handler.
func TestRegression_RepoScopeGuard_ReturnsMappedErrorOnReaderFailure(t *testing.T) {
	reader := &fakeRepoReader{err: errors.New("read model unavailable")}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/R/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"a reader failure must abort with a mapped status, not reach the handler")
}

// TestRegression_RepoScopeGuard_SkipsWhenNoRepoId proves the repo collection
// route (no :repoId) passes through without a lookup.
func TestRegression_RepoScopeGuard_SkipsWhenNoRepoId(t *testing.T) {
	reader := &fakeRepoReader{repo: &domain.Repository{ID: "R", ProjectID: "pA"}}
	r := mountRepoGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, reader.called, "a route with no :repoId must not trigger a repo lookup")
}

type fakeScopeReader struct {
	ws     domain.Workspace
	err    error
	called int
}

func (f *fakeScopeReader) Get(_ context.Context, _ string) (domain.Workspace, error) {
	f.called++
	return f.ws, f.err
}

func mountScopeGuard(reader workspaceScopeReader) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/v0/projects/:projectId/repos/:repoId")
	grp.Use(scopeWorkspaceToPath(reader))
	grp.GET("/workspaces/:wsId/thing", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	grp.GET("/workspaces", func(c *gin.Context) { c.String(http.StatusOK, "list") })
	return r
}

// TestRegression_ScopeGuard_RejectsCrossScopeWsId proves pass-3 #1: a :wsId that
// belongs to a different project/repo must 404, not leak/mutate the workspace.
func TestRegression_ScopeGuard_RejectsCrossScopeWsId(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pX", RepoID: "rX"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"a :wsId from a different project/repo must 404 rather than reach the handler")
}

// TestRegression_ScopeGuard_RejectsRepoMismatchSameProject proves the repo
// segment is enforced even when the project matches.
func TestRegression_ScopeGuard_RejectsRepoMismatchSameProject(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pA", RepoID: "rX"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestRegression_ScopeGuard_AllowsMatchingScope proves an in-scope :wsId reaches
// the handler.
func TestRegression_ScopeGuard_AllowsMatchingScope(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pA", RepoID: "rA"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestRegression_ScopeGuard_ReturnsMappedErrorOnReaderFailure proves a
// workspace-lookup error (a storage read failure, not a missing workspace) is
// mapped through libs.StatusAndMessage and aborts the chain, rather than
// falling through to the generic 404 a project/repo mismatch gets or reaching
// the handler.
func TestRegression_ScopeGuard_ReturnsMappedErrorOnReaderFailure(t *testing.T) {
	reader := &fakeScopeReader{err: errors.New("read model unavailable")}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces/W/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"a reader failure must abort with a mapped status, not reach the handler")
}

// TestRegression_ScopeGuard_SkipsWhenNoWsId proves collection/repo-level routes
// (no :wsId) pass through without a workspace lookup.
func TestRegression_ScopeGuard_SkipsWhenNoWsId(t *testing.T) {
	reader := &fakeScopeReader{ws: domain.Workspace{ID: "W", ProjectID: "pA", RepoID: "rA"}}
	r := mountScopeGuard(reader)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/pA/repos/rA/workspaces", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, reader.called, "a route with no :wsId must not trigger a workspace lookup")
}

// fakeChatWorktreeResolver stands in for the container's worktree resolver
// (usecases.WorktreeResolver): a fixed workspace or a fixed error per call,
// and the chat id it was last asked to resolve.
type fakeChatWorktreeResolver struct {
	ws       domain.Workspace
	err      error
	lastChat string
	called   int
}

func (f *fakeChatWorktreeResolver) Resolve(
	_ context.Context,
	chatID string,
) (domain.Workspace, error) {
	f.called++
	f.lastChat = chatID
	return f.ws, f.err
}

func mountChatWorktreeGuard(resolver chatWorktreeResolver) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("/v0/chats/:chatId")
	grp.Use(resolveChatWorktree(resolver))
	grp.GET("/thing", func(c *gin.Context) {
		ws, ok := reqscope.Workspace(c)
		if !ok {
			c.String(http.StatusInternalServerError, "no workspace in context")
			return
		}
		c.String(http.StatusOK, ws.ID)
	})
	return r
}

// TestRegression_ChatWorktreeGuard_ResolvesAndPopulatesContext proves a
// successful resolve stashes the workspace on the context (readable via
// reqscope.Workspace) and lets the request proceed to the handler.
func TestRegression_ChatWorktreeGuard_ResolvesAndPopulatesContext(t *testing.T) {
	resolver := &fakeChatWorktreeResolver{ws: domain.Workspace{ID: "W"}}
	r := mountChatWorktreeGuard(resolver)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/chats/chat-1/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "W", w.Body.String())
	assert.Equal(t, 1, resolver.called)
	assert.Equal(t, "chat-1", resolver.lastChat)
}

// TestRegression_ChatWorktreeGuard_NoWorktreeInAncestry404sAndAborts proves a
// chat with no worktree anywhere in its ancestry (worktree.ErrNoWorktreeInAncestry)
// 404s before the handler runs.
func TestRegression_ChatWorktreeGuard_NoWorktreeInAncestry404sAndAborts(t *testing.T) {
	resolver := &fakeChatWorktreeResolver{err: worktree.ErrNoWorktreeInAncestry}
	r := mountChatWorktreeGuard(resolver)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/chats/chat-bubble/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestRegression_ChatWorktreeGuard_ResolverErrorIsSurfacedNotSwallowed proves
// an unrelated resolve failure (an ancestry/workspace store error, not
// ErrNoWorktreeInAncestry) still aborts the request with the same
// 404-shaped envelope, rather than being swallowed and letting the request
// proceed with no workspace in context.
func TestRegression_ChatWorktreeGuard_ResolverErrorIsSurfacedNotSwallowed(t *testing.T) {
	resolver := &fakeChatWorktreeResolver{err: errors.New("store unavailable")}
	r := mountChatWorktreeGuard(resolver)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/chats/chat-1/thing", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotEqual(t, http.StatusInternalServerError, w.Code,
		"the handler must never run on a resolve error — a run would 500 from the missing context value")
}

func mountRejectEmptyPathParams() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rejectEmptyPathParams())
	r.GET("/v0/chats/:chatId/activity", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

// TestRejectEmptyPathParams_RejectsAnEmptySegment pins the fix this
// middleware exists for: gin's radix tree happily matches
// GET /v0/chats//activity against /v0/chats/:chatId/activity with chatId
// bound to "". Without this guard, an empty id would reach the handler and be
// treated as a real (if nonsensical) chat id.
func TestRejectEmptyPathParams_RejectsAnEmptySegment(t *testing.T) {
	r := mountRejectEmptyPathParams()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/chats//activity", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "chatId", "the 400 must name which param was empty")
}

// TestRejectEmptyPathParams_AllowsANonEmptySegment proves the guard is not
// simply refusing every request through it: a normal, populated :chatId still
// reaches the handler.
func TestRejectEmptyPathParams_AllowsANonEmptySegment(t *testing.T) {
	r := mountRejectEmptyPathParams()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/chats/c1/activity", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// stubClientStore stands in for store.ScopedStore[domain.Client, string]: a
// fixed set of clients, matched on TokenHash the way requireBearerAuth calls
// FindWhere.
type stubClientStore struct {
	clients []domain.Client
}

func (s *stubClientStore) FindWhere(_ context.Context, match domain.Client) ([]domain.Client, error) {
	var out []domain.Client
	for _, c := range s.clients {
		if match.TokenHash != "" && c.TokenHash == match.TokenHash {
			out = append(out, c)
		}
	}
	return out, nil
}

// TestRequireBearerAuth_NoTLS_PassesThrough proves a plaintext connection (the
// unix socket, or a loopback dev TCP bind) is out of this middleware's scope
// entirely — no header is required.
func TestRequireBearerAuth_NoTLS_PassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil) // no TLS
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "a non-TLS request must pass through unauthenticated")
}

// TestRequireBearerAuth_TLS_NoHeader_Rejects proves a TLS request with no
// Authorization header is rejected rather than treated as anonymous.
func TestRequireBearerAuth_TLS_NoHeader_Rejects(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected 401 with no Authorization header")
}

// TestRequireBearerAuth_TLS_UnknownToken_Rejects proves a bearer token with no
// matching client hash is rejected.
func TestRequireBearerAuth_TLS_UnknownToken_Rejects(t *testing.T) {
	r := gin.New()
	r.Use(requireBearerAuth(&stubClientStore{}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected 401 for an unknown token")
}

// TestRequireBearerAuth_TLS_ValidToken_Passes proves a token whose hash
// matches a known, non-revoked client authenticates and stashes the client
// id for ClientIDFrom.
func TestRequireBearerAuth_TLS_ValidToken_Passes(t *testing.T) {
	token := "abc123"
	store := &stubClientStore{clients: []domain.Client{
		{ID: "c1", TokenHash: pairing.HashToken(token)},
	}}
	r := gin.New()
	r.Use(requireBearerAuth(store))
	r.GET("/x", func(c *gin.Context) {
		assert.Equal(t, "c1", ClientIDFrom(c))
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "expected 200 for a valid token")
}

// TestRequireBearerAuth_TLS_RevokedToken_Rejects proves a revoked client's
// token stops authenticating immediately — a security property, since a
// revoke is meant to take effect on the client's very next request.
func TestRequireBearerAuth_TLS_RevokedToken_Rejects(t *testing.T) {
	token := "abc123"
	revokedAt := time.Now()
	store := &stubClientStore{clients: []domain.Client{
		{ID: "c1", TokenHash: pairing.HashToken(token), RevokedAt: &revokedAt},
	}}
	r := gin.New()
	r.Use(requireBearerAuth(store))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code, "expected 401 for a revoked client's token")
}
