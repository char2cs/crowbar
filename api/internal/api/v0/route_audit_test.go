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
		// §2.9a Threads (promoted out of /review into a first-class endpoint, W9)
		"GET " + ws + "/threads",
		"POST " + ws + "/threads",
		"GET " + ws + "/threads/:threadId",
		"PATCH " + ws + "/threads/:threadId",
		"POST " + ws + "/threads/:threadId/replies",
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
		"GET " + ws + "/terminals",
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
	const home = "/v0/projects/:projectId/home"
	return []string{
		"DELETE /v0/projects/:projectId",
		"DELETE " + repo,
		// Repo rename: the sidebar's inline repo rename, a single store write
		// that rewrites the display name + its derived avatar and broadcasts the
		// updated RepoDTO.
		"PATCH " + repo,
		"GET " + repo + "/icon",
		"PUT " + repo + "/icon",
		"DELETE " + repo + "/icon",
		"PUT " + repo + "/icon/emoji",
		"PUT " + repo + "/icon/github",
		"GET " + repo + "/branches",
		// File copy: the duplicate op the file tree's context menu drives,
		// sibling of the create/rename/delete file routes the §2.4 list carries.
		"POST " + ws + "/files/copy",
		// Branch Review's file list: the full changed-file set of the review,
		// read alongside the GET/PATCH .../review pair in §2.9.
		"GET " + ws + "/review/files",
		// Branch Review's WINDOWED diff API (diff perf phase 2). /review carries
		// the whole diff as JSON and is O(lines); these three replace it with
		// reads no single one of which is: the outline is the hunk geometry of
		// every file and nothing else (O(hunks)), patch streams ONE file's
		// unified patch as text/plain, and search is the server-side
		// find-in-diff the client can no longer do locally once it only holds a
		// window. All three resolve the diff ref exactly as /review/files does.
		"GET " + ws + "/review/outline",
		"GET " + ws + "/review/patch",
		"GET " + ws + "/review/search",
		"POST " + ws + "/lsp/didOpen",
		"POST " + ws + "/lsp/didChange",
		"POST " + ws + "/lsp/didClose",
		// Registered hierarchy/feature routes the §2 spec list did not yet
		// enumerate: the rebase-onto-parent hierarchy op (sibling of
		// merge-into-parent/reparent), the detach-holder/retry-provision
		// placeholder ops (spec §3.3/§3.5 — free a held protected branch and
		// re-provision it in place), the git-identity read, and the review-thread
		// message CRUD.
		// Branch rename: renames the workspace's branch AND relocates its
		// workspace root to the directory the new name derives, so git, the
		// filesystem and the record never disagree.
		"PATCH " + ws,
		"POST " + ws + "/rebase-onto-parent",
		"POST " + ws + "/detach-holder",
		"POST " + ws + "/retry-provision",
		"GET " + ws + "/identity",
		"DELETE " + ws + "/threads/:threadId",
		"PATCH " + ws + "/threads/:threadId/messages/:messageId",
		"DELETE " + ws + "/threads/:threadId/messages/:messageId",
		// Agentic-chat surface (00 agentic-engine spec §7): the workspace-scoped
		// REST + lifecycle WS the FE Chats tab drives, nested under the workspace
		// group (agent.Register).
		"POST " + ws + "/agent/chats",
		"GET " + ws + "/agent/chats",
		"GET " + ws + "/agent/chats/:id",
		"POST " + ws + "/agent/chats/:id/switch",
		"POST " + ws + "/agent/chats/:id/rename",
		"GET " + ws + "/agent/chats/:id/handoff",
		"DELETE " + ws + "/agent/chats/:id",
		"POST " + ws + "/agent/hooks",
		"GET " + ws + "/agent/providers",
		"GET " + ws + "/agent/ws/chats",
		// The runner model added these two and never declared them here, which is
		// exactly the drift this audit exists to catch — it caught them.
		//
		//   resume: a chat whose CLI is gone is not gone. Its ledger and its provider
		//   conversation both outlive the process, so a dormant chat can be handed a
		//   NEW runner that picks the conversation back up.
		//   runners/:segid/rename: the title path the AGENT itself calls (`crowbar chat
		//   rename`), keyed by RUNNER because the CLI knows which process it is and
		//   never which chat it currently sits on — the runner is what maps one to the
		//   other, and it keeps answering across a /clear that moves the CLI mid-turn.
		"POST " + ws + "/agent/chats/:id/resume",
		"POST " + ws + "/agent/runners/:segid/rename",
		//   stop: closing a chat TAB is not deleting the chat. The CLI is quit and
		//   the chat left DORMANT with its bound vendor conversation intact, which
		//   is exactly the state resume above exists to pick back up.
		"POST " + ws + "/agent/chats/:id/stop",
		// Provider PRIORITY + enable/disable is a GLOBAL user setting (the CLIs are
		// machine-level, not per workspace), so its write route mounts outside the
		// entity hierarchy beside /settings/terminal/profiles. It is the write
		// counterpart of the workspace-scoped enriched GET .../agent/providers.
		"PUT /v0/settings/agent/providers",
		// Repo-home-as-workspace surface: the special non-git default workspace is
		// navigable like a workspace but git-less, exposing its own files,
		// terminals, and review-thread subtrees under /projects/:projectId/home
		// (Crowbar workspace model — the repo home hosts chats/threads + a file
		// tree + terminals, but no git operations).
		"GET " + home,
		"GET " + home + "/files/tree",
		"GET " + home + "/files/content",
		"PUT " + home + "/files/content",
		"POST " + home + "/files",
		"PATCH " + home + "/files",
		"DELETE " + home + "/files",
		"GET " + home + "/files/ws",
		"GET " + home + "/terminals",
		"POST " + home + "/terminals",
		"DELETE " + home + "/terminals/:sessionId",
		"GET " + home + "/terminals/:sessionId/ws",
		"GET " + home + "/threads",
		"POST " + home + "/threads",
		"GET " + home + "/threads/:threadId",
		"PATCH " + home + "/threads/:threadId",
		"DELETE " + home + "/threads/:threadId",
		"POST " + home + "/threads/:threadId/replies",
		"PATCH " + home + "/threads/:threadId/messages/:messageId",
		"DELETE " + home + "/threads/:threadId/messages/:messageId",
		// Agentic chats re-mounted under the home group so project-home
		// workspaces get chats too (Task 6). Same handler set as the
		// workspace-scoped agent surface, each RequireHomeWorkspace-scoped.
		"POST " + home + "/agent/chats",
		"GET " + home + "/agent/chats",
		"GET " + home + "/agent/chats/:id",
		"POST " + home + "/agent/chats/:id/switch",
		"POST " + home + "/agent/chats/:id/rename",
		"GET " + home + "/agent/chats/:id/handoff",
		"DELETE " + home + "/agent/chats/:id",
		"POST " + home + "/agent/hooks",
		"GET " + home + "/agent/providers",
		"GET " + home + "/agent/ws/chats",
		// The repo home hosts chats like any workspace, so it gets the runner model's
		// two new routes too — same pair, same reasons as the ws block above. A chat in
		// the home is still a chat: its CLI can die and be resumed, and the agent
		// running in it still has to be able to name it.
		"POST " + home + "/agent/chats/:id/resume",
		"POST " + home + "/agent/runners/:segid/rename",
		// And the same close-is-not-delete stop, for the same reason: a home chat's
		// tab closes exactly like any other chat's.
		"POST " + home + "/agent/chats/:id/stop",
		// The home hosts a file tree, so it hosts the file tree's duplicate op too.
		"POST " + home + "/files/copy",
		// The daemon's timing-ring read/arm seam. Process-wide, not scoped to a
		// project/repo/workspace, so it mounts top-level beside
		// /system/prerequisites rather than inside the entity hierarchy.
		"GET /v0/system/perf",
		"POST /v0/system/perf",
		// The client's UI state, held daemon-side so a client with no local
		// persistence recovers its layout on boot. Addressed by an explicit
		// ?scope= key ("global", "repo:<id>", "workspace:<id>") rather than by
		// path position, so it mounts top-level beside the other /settings/*
		// groups instead of forking into three entity-nested routes.
		"GET /v0/settings/ui",
		"PUT /v0/settings/ui",
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

	// w1 must exist under p1/r1 so the scope guard admits the :wsId routes below.
	seedWorkspace(t, tc, "w1")

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

	// w1 must exist under p1/r1 so the scope guard admits the :wsId routes below.
	seedWorkspace(t, tc, "w1")

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
