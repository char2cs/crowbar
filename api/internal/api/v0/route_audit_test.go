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
		// Project reorder: the sidebar's manual project order, a single store
		// write that densifies the list and broadcasts every row it shifted.
		"PATCH /v0/projects/:projectId",
		"DELETE " + repo,
		// Repo patch: the sidebar's inline repo rename, its manual order within a
		// project, and its move BETWEEN projects — one partial-update endpoint,
		// because a drag can do more than one of them at once. It rewrites the
		// display name + its derived avatar and broadcasts the updated RepoDTO.
		"PATCH " + repo,
		// Folder CRUD: the sidebar's organisation layer (folders are repo-scoped
		// and hold no worktree). The list GET dual-serves the Folders WS stream.
		"GET " + repo + "/folders",
		"POST " + repo + "/folders",
		"PATCH " + repo + "/folders/:folderId",
		"DELETE " + repo + "/folders/:folderId",
		// The repo's open-PR head->base graph, the import dialog's parent hint.
		"GET " + repo + "/pull-requests",
		// Batch branch import: adopts a set of remote branches as managed
		// workspaces in one call, PR-parented up to a protected root.
		"POST " + repo + "/workspaces/import",
		"GET " + repo + "/icon",
		"PUT " + repo + "/icon",
		"DELETE " + repo + "/icon",
		"PUT " + repo + "/icon/emoji",
		"PUT " + repo + "/icon/github",
		// Project icon: the same three states a repo's has (uploaded image,
		// emoji, or the client's default mark) on the same routes one level up.
		// No /icon/github counterpart — that one reads the repo's `origin`
		// remote for an owner avatar, and a project has no remote of its own.
		"GET /v0/projects/:projectId/icon",
		"PUT /v0/projects/:projectId/icon",
		"DELETE /v0/projects/:projectId/icon",
		"PUT /v0/projects/:projectId/icon/emoji",
		// The user's own lock decision for a workspace, which outranks the
		// provider's protected flag and survives the next poll. A verb route
		// rather than a PATCH field: it is one aggregate write with no git in it,
		// and it answers synchronously so a refusal (the project home, a
		// placeholder with no worktree) reaches the menu that fired it.
		"POST " + ws + "/lock",
		"GET " + repo + "/branches",
		// Two routes that shipped without ever being declared here — the exact
		// drift this audit exists to catch, caught late because the audit is
		// behind the `integration` build tag and so does not run in the default
		// `go test ./...` sweep.
		//
		//   pull-requests: the repo's open PR list, read by the sidebar beside
		//   the branch list above.
		//   workspaces/import: adopting an EXISTING branch as a workspace, the
		//   sibling of POST .../workspaces (which creates a new branch). It is
		//   repo-scoped, not workspace-scoped: there is no :wsId until it
		//   returns.
		"GET " + repo + "/pull-requests",
		"POST " + repo + "/workspaces/import",
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
		"POST " + ws + "/chats",
		"GET " + ws + "/chats",
		"GET " + ws + "/chats/:id",
		"POST " + ws + "/chats/:id/switch",
		"POST " + ws + "/chats/:id/rename",
		"GET " + ws + "/chats/:id/handoff",
		"DELETE " + ws + "/chats/:id",
		// Chat placement: where a chat hangs in the Chats tree and where it sits
		// among its siblings. A route of its own rather than a field on the chat
		// PATCH-equivalents, because it writes something different in kind — a
		// chat's parent IS its context lineage, so this is what turns a standalone
		// chat into a THREAD of another and back.
		"PATCH " + ws + "/chats/:id/placement",
		// Chat folder CRUD: the Chats panel's organisation layer. Workspace-scoped,
		// and sharing ONE dense sibling space with the chats above — a folder and a
		// chat interleave at every level, which is why the placement route above and
		// these four are the two halves of the same gesture.
		"GET " + ws + "/chats/folders",
		"POST " + ws + "/chats/folders",
		"PATCH " + ws + "/chats/folders/:folderId",
		"DELETE " + ws + "/chats/folders/:folderId",
		"POST " + ws + "/chats/hooks",
		"GET " + ws + "/chats/providers",
		"GET " + ws + "/chats/ws",
		// The runner model added resume and never declared it here, which is exactly
		// the drift this audit exists to catch — it caught it. A chat whose CLI is gone
		// is not gone: its ledger and its provider conversation both outlive the
		// process, so a dormant chat can be handed a NEW runner that picks the
		// conversation back up.
		"POST " + ws + "/chats/:id/resume",
		// The two directions of the native-TUI/React-terminal split: SwitchToTerminal
		// hands the live PTY to the terminal pane, SwitchToNative reverses it. Never
		// declared here — the exact drift this audit exists to catch.
		"POST " + ws + "/chats/:id/switch-to-terminal",
		"POST " + ws + "/chats/:id/switch-to-native",
		// The chat's own guarded/trusted/full-auto dial, PUT-only: the read side is
		// folded into the chat GET, so only the write leg is a route of its own.
		"PUT " + ws + "/chats/:id/permission-level",
		//   runners/:segid/mcp: the agent's own tool surface, spoken as MCP. Keyed by
		//   RUNNER because the CLI knows which process it is and never which chat it
		//   currently sits on — the runner is what maps one to the other, and it keeps
		//   answering across a /clear that moves the CLI mid-turn. Plus one reason that
		//   applies only here: this is the route the agent's process itself calls, so
		//   its authority has to come from the per-boot token bound to that runner and
		//   never from a URL the agent is free to compose.
		//
		//   Its runner-keyed sibling, .../runners/:segid/rename, is deliberately GONE.
		//   It existed for a shell command the agent was asked to retype; titling is a
		//   tool on this MCP surface now, and a second path competed with it.
		"POST " + ws + "/chats/runners/:segid/mcp",
		//   stop: closing a chat TAB is not deleting the chat. The CLI is quit and
		//   the chat left DORMANT with its bound vendor conversation intact, which
		//   is exactly the state resume above exists to pick back up.
		"POST " + ws + "/chats/:id/stop",
		"POST " + ws + "/chats/:id/compact",
		// The read surface the Chats tab polls: the activity feed, a tool call's
		// full payload, open choices, message history, and provider telemetry.
		"GET " + ws + "/chats/:id/activity",
		"GET " + ws + "/chats/:id/activity/:toolId/payload",
		"GET " + ws + "/chats/:id/choices",
		"GET " + ws + "/chats/:id/messages",
		"GET " + ws + "/chats/:id/telemetry",
		// The provider's own slash-command list, so the composer can autocomplete
		// commands the CLI itself defines.
		"GET " + ws + "/chats/:id/slash-catalog",
		// The chat's sticky model/reasoning-effort choice.
		"PATCH " + ws + "/chats/:id/selection",
		// A human deciding a question the agent put to them mid-turn.
		"POST " + ws + "/chats/:id/choices/:choiceId/answer",
		// Submitting the user's own text into the chat.
		"POST " + ws + "/chats/:id/prompts",
		// The answer channel's other two legs (routes.go): the in-PTY relay parking
		// alive while the provider's gate stays open, and what it reports when the
		// provider decided at the terminal instead.
		"POST " + ws + "/chats/hooks/await",
		"POST " + ws + "/chats/hooks/abandon",
		// Provider PRIORITY + enable/disable is a GLOBAL user setting (the CLIs are
		// machine-level, not per workspace), so its write route mounts outside the
		// entity hierarchy beside /settings/terminal/profiles. It is the write
		// counterpart of the workspace-scoped enriched GET .../chats/providers.
		"PUT /v0/settings/chat/providers",
		// The install-wide default permission level new chats seed from, read and
		// written beside the provider-priority setting above for the same reason:
		// a global CLI-level dial, not a per-workspace one.
		"GET /v0/settings/chat/permission-level",
		"PUT /v0/settings/chat/permission-level",
		// The host terminal's light/dark colours, and a GLOBAL setting for the same
		// reason: one Crowbar window renders every session, so there is one theme, and
		// it must be known BEFORE any session exists. The daemon seeds it into each PTY
		// at birth so a CLI that detects its theme by querying the background (OSC 10/11)
		// reads the truth on its first frame — the per-session push cannot cover that,
		// since the process is already running by the time a client can attach.
		"PUT /v0/settings/terminal/theme",
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
		"POST " + home + "/chats",
		"GET " + home + "/chats",
		"GET " + home + "/chats/:id",
		"POST " + home + "/chats/:id/switch",
		"POST " + home + "/chats/:id/rename",
		"GET " + home + "/chats/:id/handoff",
		"DELETE " + home + "/chats/:id",
		"POST " + home + "/chats/hooks",
		"GET " + home + "/chats/providers",
		"GET " + home + "/chats/ws",
		// The repo home hosts chats like any workspace, so it gets the runner model's
		// resume too — same reason as the ws block above. A chat in the home is still a
		// chat: its CLI can die and be resumed.
		"POST " + home + "/chats/:id/resume",
		// Same native-TUI/React-terminal split and permission-level write, mounted
		// on the home group for the same reason as the rest of this block.
		"POST " + home + "/chats/:id/switch-to-terminal",
		"POST " + home + "/chats/:id/switch-to-native",
		"PUT " + home + "/chats/:id/permission-level",
		// An agent in a home chat has the same tools as one anywhere else, so the
		// MCP seam is mounted here too — and, like the ws block, WITHOUT the retired
		// runner-keyed rename route beside it.
		"POST " + home + "/chats/runners/:segid/mcp",
		// And the same close-is-not-delete stop, for the same reason: a home chat's
		// tab closes exactly like any other chat's.
		"POST " + home + "/chats/:id/stop",
		"POST " + home + "/chats/:id/compact",
		// Placement and chat FOLDERS re-mounted on the home group. This is the
		// mount that matters most: the project home accumulates more chats than any
		// worktree workspace, so the surface that most needs somewhere to put them
		// is exactly the one a single mount would have left without folders.
		"PATCH " + home + "/chats/:id/placement",
		"GET " + home + "/chats/folders",
		"POST " + home + "/chats/folders",
		"PATCH " + home + "/chats/folders/:folderId",
		"DELETE " + home + "/chats/folders/:folderId",
		// The same read surface, slash-catalog, selection, answer, and prompts
		// routes, mounted on the home group for the same reason as the rest of this
		// block: a chat in the project home behaves like a chat anywhere else.
		"GET " + home + "/chats/:id/activity",
		"GET " + home + "/chats/:id/activity/:toolId/payload",
		"GET " + home + "/chats/:id/choices",
		"GET " + home + "/chats/:id/messages",
		"GET " + home + "/chats/:id/telemetry",
		"GET " + home + "/chats/:id/slash-catalog",
		"PATCH " + home + "/chats/:id/selection",
		"POST " + home + "/chats/:id/choices/:choiceId/answer",
		"POST " + home + "/chats/:id/prompts",
		// And the same answer-channel pair, for the same reason.
		"POST " + home + "/chats/hooks/await",
		"POST " + home + "/chats/hooks/abandon",
		// The home hosts a file tree, so it hosts the file tree's duplicate op too.
		"POST " + home + "/files/copy",
		// The daemon's timing-ring read/arm seam. Process-wide, not scoped to a
		// project/repo/workspace, so it mounts top-level beside
		// /system/prerequisites rather than inside the entity hierarchy.
		"GET /v0/system/perf",
		"POST /v0/system/perf",
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
