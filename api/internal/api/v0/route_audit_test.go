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

// specRoutes is every route in 02 §2 + §3, one per spec line, re-nested under
// the hierarchical /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/...
// prefix (spec §3). Health, system, and the terminal profiles CRUD remain
// top-level (outside /projects). The dedicated /ws/* routes are GONE (W7-2):
// the entity list+detail GET routes dual-serve REST/WS, and the raw/diagnostic
// streams are co-located as .../files/ws, .../lsp/ws, .../terminals/:sessionId/ws.
func specRoutes() []string {
	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	return []string{
		// §2.1 Projects & Repositories
		"GET /v0/projects",
		"POST /v0/projects",
		"GET /v0/projects/:projectId",
		"GET /v0/projects/:projectId/repos",
		"POST /v0/projects/:projectId/repos",
		"GET " + repo,
		// §2.2 Workspaces (+hierarchy)
		"GET /v0/projects/:projectId/repos/:repoId/workspaces",
		"GET " + ws,
		"POST /v0/projects/:projectId/repos/:repoId/workspaces",
		"DELETE " + ws,
		"POST " + ws + "/sync",
		"POST " + ws + "/merge-into-parent",
		"POST " + ws + "/reparent",
		// §2.3 Chats — chat WebSocket surface removed per D11; chat domain, repo
		// CRUD, and usecase remain dormant TODO (routes not remounted in this PR).
		// §2.4 Files
		"GET " + ws + "/files/tree",
		"GET " + ws + "/files/content",
		"PUT " + ws + "/files/content",
		"POST " + ws + "/files",
		"PATCH " + ws + "/files",
		"DELETE " + ws + "/files",
		// §2.5 Editor / LSP (+blame)
		"GET " + ws + "/blame",
		"POST " + ws + "/lsp/completion",
		"POST " + ws + "/lsp/hover",
		"POST " + ws + "/lsp/definition",
		"POST " + ws + "/lsp/references",
		"POST " + ws + "/lsp/rename",
		"POST " + ws + "/lsp/codeAction",
		"POST " + ws + "/lsp/documentSymbol",
		"GET " + ws + "/lsp/diagnostics",
		// §2.6 Git — Read
		"GET " + ws + "/git/status",
		"GET " + ws + "/git/log",
		"GET " + ws + "/git/diff",
		"GET " + ws + "/git/blame",
		"GET " + ws + "/git/branches",
		"GET " + ws + "/git/stashes",
		"GET " + ws + "/git/conflicts",
		"GET " + ws + "/git/conflict-hunks",
		"GET " + ws + "/git/commit-diff",
		// §2.7 Git — Write
		"POST " + ws + "/git/stage",
		"POST " + ws + "/git/stage-hunk",
		"POST " + ws + "/git/unstage",
		"POST " + ws + "/git/unstage-hunk",
		"POST " + ws + "/git/discard",
		"POST " + ws + "/git/commit",
		"POST " + ws + "/git/push",
		"POST " + ws + "/git/pull",
		"POST " + ws + "/git/fetch",
		"POST " + ws + "/git/branches",
		"PATCH " + ws + "/git/branches",
		"DELETE " + ws + "/git/branches",
		"POST " + ws + "/git/switch",
		"POST " + ws + "/git/stash",
		"POST " + ws + "/git/stash-apply",
		"POST " + ws + "/git/stash-pop",
		"DELETE " + ws + "/git/stash",
		"POST " + ws + "/git/reset",
		"POST " + ws + "/git/merge",
		"POST " + ws + "/git/rebase",
		"POST " + ws + "/git/resolve-hunk",
		// §2.8 Conflicts (+operation)
		"POST " + ws + "/git/operation/continue",
		"POST " + ws + "/git/operation/abort",
		// §2.9 Review
		"GET " + ws + "/review",
		"PATCH " + ws + "/review",
		"POST " + ws + "/review/threads",
		"POST " + ws + "/review/threads/:id/reply",
		"PATCH " + ws + "/review/threads/:id",
		// §2.9b Provider (read-only)
		"GET " + ws + "/provider",
		"GET " + repo + "/protected-branches",
		// §2.10 Search
		"POST " + ws + "/search",
		"POST " + ws + "/search/replace",
		// §2.11 Terminal (+profiles)
		"GET /v0/settings/terminal/profiles",
		"GET /v0/settings/terminal/profiles/:id",
		"POST /v0/settings/terminal/profiles",
		"PUT /v0/settings/terminal/profiles/:id",
		"DELETE /v0/settings/terminal/profiles/:id",
		"POST " + ws + "/terminals",
		"DELETE " + ws + "/terminals/:sessionId",
		// §2.12 System
		"GET /v0/system/prerequisites",
		// §2.13 Health
		"GET /v0/health",
		// §3 WebSocket endpoints — co-located on the nested tree (W7-2). The
		// entity list+detail GET routes dual-serve REST/WS in place (no separate
		// path); the raw/diagnostic streams hang off their workspace subtree.
		"GET " + ws + "/files/ws",
		"GET " + ws + "/lsp/ws",
		"GET " + ws + "/terminals/:sessionId/ws",
	}
}

// extraRoutes is the documented superset registered beyond the core spec:
// repo icon management, repo branch listing, project deletion (record-only
// cascade), and LSP document-sync notifications (04 §3, 10).
func extraRoutes() []string {
	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	return []string{
		"DELETE /v0/projects/:projectId",
		"DELETE " + repo,
		"GET " + repo + "/icon",
		"PUT " + repo + "/icon",
		"DELETE " + repo + "/icon",
		"PUT " + repo + "/icon/emoji",
		"PUT " + repo + "/icon/github",
		"GET " + repo + "/branches",
		"POST " + ws + "/lsp/didOpen",
		"POST " + ws + "/lsp/didChange",
		"POST " + ws + "/lsp/didClose",
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

// TestRouteAudit_NoLegacyWsRoutes asserts the five dedicated /ws/* routes are
// GONE from the registered surface (W7-2): /ws/workspaces, /ws/git, /ws/files,
// /ws/lsp, and /ws/terminals/:sessionId were folded into the dual-served entity
// GET routes and the co-located .../files/ws, .../lsp/ws, .../terminals/:id/ws.
func TestRouteAudit_NoLegacyWsRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	got := registeredRoutes(t)

	const repo = "/v0/projects/:projectId/repos/:repoId"
	legacy := []string{
		"GET " + repo + "/ws/workspaces",
		"GET " + repo + "/ws/git",
		"GET " + repo + "/ws/files",
		"GET " + repo + "/ws/lsp",
		"GET " + repo + "/ws/terminals/:sessionId",
	}
	for _, r := range legacy {
		_, ok := got[r]
		assert.Falsef(t, ok, "legacy /ws/* route must be removed: %s", r)
	}
	// Any residual /ws/ segment anywhere in the surface is a regression.
	for r := range got {
		assert.NotContainsf(t, r, "/ws/workspaces", "legacy ws route present: %s", r)
		assert.NotContainsf(t, r, "/ws/git", "legacy ws route present: %s", r)
		assert.NotContainsf(t, r, "/ws/lsp", "legacy ws route present: %s", r)
	}
}

// TestRouteAudit_DualServe_RestMode proves the dual-served live-read routes
// answer a plain (non-Upgrade) GET on REST — the complement of the WS-upgrade
// proofs below, so both modes of every route are covered. It exercises the
// entity list+detail routes (projects, projects/:id, repos, repos/:id,
// workspaces, workspaces/:id) plus git/status.
func TestRouteAudit_DualServe_RestMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	for _, path := range []string{
		"/v0/projects",
		"/v0/projects/p1",
		"/v0/projects/p1/repos",
		"/v0/projects/p1/repos/r1",
		"/v0/projects/p1/repos/r1/workspaces",
		"/v0/projects/p1/repos/r1/workspaces/w1",
		"/v0/projects/p1/repos/r1/workspaces/w1/git/status",
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

// TestRouteAudit_DualServe_WsMode proves every dual-served route upgrades to a
// live WebSocket stream when the request carries Upgrade: websocket — including
// the new project/repo/workspace detail routes (W7-2).
func TestRouteAudit_DualServe_WsMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	for _, path := range []string{
		"/v0/projects",
		"/v0/projects/p1",
		"/v0/projects/p1/repos",
		"/v0/projects/p1/repos/r1",
		"/v0/projects/p1/repos/r1/workspaces",
		"/v0/projects/p1/repos/r1/workspaces/w1",
		"/v0/projects/p1/repos/r1/workspaces/w1/git/status",
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
