//go:build integration

package v0_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
)

// expectedRoutes is the canonical method+path set the v0 surface must register,
// audited against docs/specs/v0/02-api-surface.md §2 (REST) + §3 (WS). It is the
// regression guard for T18: dropping or renaming any route breaks this test.
//
// Spec param names (:id, :childId) are normalised to the registered names
// (:wsId, :sessionId, :branch); the routes resolve identically. Documented
// supersets beyond the spec are listed in extraRoutes.
func expectedRoutes() map[string]struct{} {
	out := map[string]struct{}{}
	for _, r := range append(append([]string{}, specRoutes()...), extraRoutes()...) {
		out[r] = struct{}{}
	}
	return out
}

// specRoutes is every route in 02 §2 + §3, one per spec line.
func specRoutes() []string {
	return []string{
		// §2.1 Projects & Repositories
		"GET /v0/projects",
		"POST /v0/projects",
		"GET /v0/projects/:id",
		"GET /v0/repos",
		"POST /v0/repos",
		"GET /v0/repos/:id",
		// §2.2 Workspaces (+hierarchy)
		"GET /v0/workspaces",
		"GET /v0/workspaces/:wsId",
		"POST /v0/workspaces",
		"DELETE /v0/workspaces/:wsId",
		"POST /v0/workspaces/:wsId/sync",
		"POST /v0/workspaces/:wsId/merge-into-parent",
		"POST /v0/workspaces/:wsId/reparent",
		// §2.3 Chats
		"GET /v0/workspaces/:wsId/chats",
		"POST /v0/workspaces/:wsId/chats",
		"POST /v0/chats/:id/fork",
		"PATCH /v0/chats/:id",
		"DELETE /v0/chats/:id",
		// §2.4 Files
		"GET /v0/workspaces/:wsId/files/tree",
		"GET /v0/workspaces/:wsId/files/content",
		"PUT /v0/workspaces/:wsId/files/content",
		"POST /v0/workspaces/:wsId/files",
		"PATCH /v0/workspaces/:wsId/files",
		"DELETE /v0/workspaces/:wsId/files",
		// §2.5 Editor / LSP (+blame)
		"GET /v0/workspaces/:wsId/blame",
		"POST /v0/workspaces/:wsId/lsp/completion",
		"POST /v0/workspaces/:wsId/lsp/hover",
		"POST /v0/workspaces/:wsId/lsp/definition",
		"POST /v0/workspaces/:wsId/lsp/references",
		"POST /v0/workspaces/:wsId/lsp/rename",
		"POST /v0/workspaces/:wsId/lsp/codeAction",
		"POST /v0/workspaces/:wsId/lsp/documentSymbol",
		"GET /v0/workspaces/:wsId/lsp/diagnostics",
		// §2.6 Git — Read
		"GET /v0/workspaces/:wsId/git/status",
		"GET /v0/workspaces/:wsId/git/log",
		"GET /v0/workspaces/:wsId/git/diff",
		"GET /v0/workspaces/:wsId/git/blame",
		"GET /v0/workspaces/:wsId/git/branches",
		"GET /v0/workspaces/:wsId/git/stashes",
		"GET /v0/workspaces/:wsId/git/conflicts",
		"GET /v0/workspaces/:wsId/git/conflict-hunks",
		"GET /v0/workspaces/:wsId/git/commit-diff",
		// §2.7 Git — Write
		"POST /v0/workspaces/:wsId/git/stage",
		"POST /v0/workspaces/:wsId/git/stage-hunk",
		"POST /v0/workspaces/:wsId/git/unstage",
		"POST /v0/workspaces/:wsId/git/unstage-hunk",
		"POST /v0/workspaces/:wsId/git/discard",
		"POST /v0/workspaces/:wsId/git/commit",
		"POST /v0/workspaces/:wsId/git/push",
		"POST /v0/workspaces/:wsId/git/pull",
		"POST /v0/workspaces/:wsId/git/fetch",
		"POST /v0/workspaces/:wsId/git/branches",
		"PATCH /v0/workspaces/:wsId/git/branches",
		"DELETE /v0/workspaces/:wsId/git/branches",
		"POST /v0/workspaces/:wsId/git/switch",
		"POST /v0/workspaces/:wsId/git/stash",
		"POST /v0/workspaces/:wsId/git/stash-apply",
		"POST /v0/workspaces/:wsId/git/stash-pop",
		"DELETE /v0/workspaces/:wsId/git/stash",
		"POST /v0/workspaces/:wsId/git/reset",
		"POST /v0/workspaces/:wsId/git/merge",
		"POST /v0/workspaces/:wsId/git/rebase",
		"POST /v0/workspaces/:wsId/git/resolve-hunk",
		// §2.8 Conflicts (+operation)
		"POST /v0/workspaces/:wsId/git/operation/continue",
		"POST /v0/workspaces/:wsId/git/operation/abort",
		// §2.9 Review
		"GET /v0/workspaces/:wsId/review",
		"PATCH /v0/workspaces/:wsId/review",
		"POST /v0/workspaces/:wsId/review/threads",
		"POST /v0/workspaces/:wsId/review/threads/:id/reply",
		"PATCH /v0/workspaces/:wsId/review/threads/:id",
		// §2.9b Provider (read-only)
		"GET /v0/workspaces/:wsId/provider",
		"GET /v0/repos/:id/protected-branches",
		// §2.10 Search
		"POST /v0/workspaces/:wsId/search",
		"POST /v0/workspaces/:wsId/search/replace",
		// §2.11 Terminal (+profiles)
		"GET /v0/settings/terminal/profiles",
		"GET /v0/settings/terminal/profiles/:id",
		"POST /v0/settings/terminal/profiles",
		"PUT /v0/settings/terminal/profiles/:id",
		"DELETE /v0/settings/terminal/profiles/:id",
		"POST /v0/workspaces/:wsId/terminals",
		"DELETE /v0/terminals/:sessionId",
		// §2.13 Health
		"GET /v0/health",
		// §3 WebSocket endpoints
		"GET /v0/ws/workspaces",
		"GET /v0/ws/chats",
		"GET /v0/ws/git",
		"GET /v0/ws/files",
		"GET /v0/ws/lsp",
		"GET /v0/ws/terminals/:sessionId",
		"GET /v0/ws/chats/:chatId/stream",
	}
}

// extraRoutes is the documented superset registered beyond the core spec:
// project deletion (record-only cascade) and LSP document-sync notifications
// (04 §3, 10).
func extraRoutes() []string {
	return []string{
		"DELETE /v0/projects/:id",
		"POST /v0/workspaces/:wsId/lsp/didOpen",
		"POST /v0/workspaces/:wsId/lsp/didChange",
		"POST /v0/workspaces/:wsId/lsp/didClose",
	}
}

func registeredRoutes(
	t *testing.T,
) map[string]struct{} {
	t.Helper()
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	out := map[string]struct{}{}
	for _, rt := range r.Routes() {
		out[rt.Method+" "+rt.Path] = struct{}{}
	}
	return out
}

// TestRouteAudit_AllSpecRoutesRegistered asserts every 02 §2/§3 route resolves
// to a registered method+path, and that the registered surface contains no
// route outside the spec∪documented-superset set. It is the regression guard:
// dropping, renaming, or adding an undocumented route fails here.
func TestRouteAudit_AllSpecRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	got := registeredRoutes(t)
	want := expectedRoutes()

	for r := range want {
		_, ok := got[r]
		assert.Truef(t, ok, "spec route not registered: %s", r)
	}
	for r := range got {
		_, ok := want[r]
		assert.Truef(t, ok, "registered route not in spec/superset: %s", r)
	}
	assert.Len(t, got, len(want), "registered route count drifted from expected set")
}

// TestRouteAudit_DualServe_RestMode proves the two dual-served live-read routes
// answer a plain (non-Upgrade) GET on REST — the complement of the WS-upgrade
// proofs in container_test.go, so both modes of both routes are covered.
func TestRouteAudit_DualServe_RestMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	for _, path := range []string{
		"/v0/workspaces",
		"/v0/workspaces/w1/git/status",
	} {
		resp, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		assert.NotEqual(
			t,
			http.StatusSwitchingProtocols,
			resp.StatusCode,
			"plain GET must serve REST, not upgrade: %s",
			path,
		)
	}
}

// TestRouteAudit_DualServe_WsMode proves both dual-served routes upgrade to a
// live WebSocket stream when the request carries Upgrade: websocket.
func TestRouteAudit_DualServe_WsMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	for _, path := range []string{
		"/v0/ws/workspaces",
		"/v0/workspaces/w1/git/status",
	} {
		url := "ws" + srv.URL[len("http"):] + path
		conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
		if resp != nil {
			_ = resp.Body.Close()
		}
		require.NoErrorf(t, err, "upgrade must succeed: %s", path)
		_ = conn.Close()
	}
}
