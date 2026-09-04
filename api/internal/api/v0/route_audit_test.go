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
// top-level (outside /projects), and the terminal SESSION routes now hang off
// the flat /v0/chats/:chatId prefix. The dedicated /ws/* routes are GONE
// (W7-2): the entity list+detail GET routes dual-serve REST/WS, and the
// raw/diagnostic streams are co-located as .../files/ws, .../lsp/ws, and
// /v0/chats/:chatId/terminals/:sessionId/ws.
func specRoutes() []string {
	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	// chat is the flat chat-scoped prefix (spec §7.1): chat ids are globally
	// unique, so nothing below it re-nests under a project/repo pair.
	const chat = "/v0/chats/:chatId"
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
		// §2.4 Files, chat-scoped (chat-scoped API spec §4.2's SHARED bucket,
		// §8 step 4). The SAME routes as the workspace-scoped block above, and
		// both are genuinely live: the flat chat prefix is where files is
		// addressed from now on, and the workspace prefix is simply not retired
		// until §8 step 6. Listing both is what makes THIS audit the thing that
		// notices when one of them gains or loses a route the other did not.
		"GET " + chat + "/files/tree",
		"GET " + chat + "/files/content",
		"PUT " + chat + "/files/content",
		"POST " + chat + "/files",
		"PATCH " + chat + "/files",
		"DELETE " + chat + "/files",
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
		// §2.5 Editor / LSP, chat-scoped (chat-scoped API spec §4.2's OWNED
		// bucket, §8 step 5). The SAME routes as the workspace-scoped block
		// above, and both are genuinely live: the flat chat prefix is where
		// editor/LSP is addressed from now on, and the workspace prefix is
		// simply not retired until §8 step 6. Listing both is what makes THIS
		// audit the thing that notices when one of them gains or loses a
		// route the other did not.
		"GET " + chat + "/blame",
		"POST " + chat + "/lsp/completion",
		"POST " + chat + "/lsp/hover",
		"POST " + chat + "/lsp/definition",
		"POST " + chat + "/lsp/references",
		"POST " + chat + "/lsp/rename",
		"POST " + chat + "/lsp/codeAction",
		"POST " + chat + "/lsp/documentSymbol",
		"GET " + chat + "/lsp/diagnostics",
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
		// §2.8 Git, chat-scoped (chat-scoped API spec §4.2's SHARED bucket,
		// §8 step 4). The SAME 32 routes as the workspace-scoped block above,
		// and both are genuinely live: the flat chat prefix is where git is
		// addressed from now on, and the workspace prefix is simply not retired
		// until §8 step 6 deletes the workspaces/home groups wholesale. Listing
		// both is what makes THIS audit the thing that notices when one of them
		// gains or loses a route the other did not.
		"GET " + chat + "/git/status",
		"GET " + chat + "/git/log",
		"GET " + chat + "/git/diff",
		"GET " + chat + "/git/blame",
		"GET " + chat + "/git/branches",
		"GET " + chat + "/git/stashes",
		"GET " + chat + "/git/conflicts",
		"GET " + chat + "/git/conflict-hunks",
		"GET " + chat + "/git/commit-diff",
		"POST " + chat + "/git/stage",
		"POST " + chat + "/git/stage-hunk",
		"POST " + chat + "/git/unstage",
		"POST " + chat + "/git/unstage-hunk",
		"POST " + chat + "/git/discard",
		"POST " + chat + "/git/commit",
		"POST " + chat + "/git/push",
		"POST " + chat + "/git/pull",
		"POST " + chat + "/git/fetch",
		"POST " + chat + "/git/branches",
		"PATCH " + chat + "/git/branches",
		"DELETE " + chat + "/git/branches",
		"POST " + chat + "/git/switch",
		"POST " + chat + "/git/stash",
		"POST " + chat + "/git/stash-apply",
		"POST " + chat + "/git/stash-pop",
		"DELETE " + chat + "/git/stash",
		"POST " + chat + "/git/reset",
		"POST " + chat + "/git/merge",
		"POST " + chat + "/git/rebase",
		"POST " + chat + "/git/resolve-hunk",
		"POST " + chat + "/git/operation/continue",
		"POST " + chat + "/git/operation/abort",
		// §2.9 Review
		"GET " + ws + "/review",
		"PATCH " + ws + "/review",
		// §2.9 Review, chat-scoped (chat-scoped API spec §4.2's SHARED bucket,
		// §8 step 4c). The SAME routes as the workspace-scoped pair above, and
		// both are genuinely live for the same reason git's chat-scoped block
		// is: the flat chat prefix is where review is addressed from now on,
		// and the workspace prefix is simply not retired until §8 step 6.
		"GET " + chat + "/review",
		"PATCH " + chat + "/review",
		// §2.9a Threads (promoted out of /review into a first-class endpoint, W9)
		"GET " + ws + "/threads",
		"POST " + ws + "/threads",
		"GET " + ws + "/threads/:threadId",
		"PATCH " + ws + "/threads/:threadId",
		"POST " + ws + "/threads/:threadId/replies",
		// §2.9b Provider (read-only)
		"GET " + ws + "/provider",
		"GET " + repo + "/protected-branches",
		// §2.9b Provider, chat-scoped (chat-scoped API spec §4.2's OWNED
		// bucket, §8 step 5). The SAME State route as the workspace-scoped one
		// above, and both are genuinely live for the same reason editor/LSP's
		// chat-scoped block is. /protected-branches does NOT move — spec §4.2
		// is explicit that it is repo-level, not worktree-owned.
		"GET " + chat + "/provider",
		// The per-CHAT lifecycle stream: the same agent-chat broadcaster the
		// repo-scoped .../chats/ws mount serves, narrowed by agentChatDef's
		// chatId filter to one chat.
		//
		// It is the chat-scoped replacement for watching ONE workspace's stream,
		// and it carries that stream's SIDE EFFECT as well as its frames:
		// subscribing to a single workspace is what starts the daemon's provider
		// poll, and this mount resolves a workspace (chatScoped's own
		// resolveChatWorktree) where the repo-wide list scope resolves none.
		"GET " + chat + "/ws",
		// §2.10 Search
		"POST " + ws + "/search",
		"POST " + ws + "/search/replace",
		// §2.10 Search, chat-scoped (chat-scoped API spec §4.2's SHARED bucket,
		// §8 step 4c). The SAME routes as the workspace-scoped pair above; see
		// the review chat-scoped comment above for why both are live.
		"POST " + chat + "/search",
		"POST " + chat + "/search/replace",
		// §2.11 Terminal (+profiles). The profile CRUD is a global user setting
		// and stays top-level. The session routes moved onto the flat
		// /v0/chats/:chatId prefix (chat-scoped API spec §8 step 3): a terminal
		// is owned by ONE chat, so the chat is the only id in the path.
		"GET /v0/settings/terminal/profiles",
		"GET /v0/settings/terminal/profiles/:id",
		"POST /v0/settings/terminal/profiles",
		"PUT /v0/settings/terminal/profiles/:id",
		"DELETE /v0/settings/terminal/profiles/:id",
		"GET " + chat + "/terminals",
		"POST " + chat + "/terminals",
		"DELETE " + chat + "/terminals/:sessionId",
		// §2.12 System
		"GET /v0/system/prerequisites",
		// §2.13 Health
		"GET /v0/health",
		// §3 WebSocket endpoints — co-located on the nested tree (W7-2). The
		// entity list+detail GET routes dual-serve REST/WS in place (no separate
		// path); the raw/diagnostic streams hang off their workspace subtree.
		"GET " + ws + "/files/ws",
		// The same live file-change stream, chat-scoped: one Broadcaster, one
		// StreamDef, two ways of naming a subscriber's scope (container.go
		// filesDef).
		"GET " + chat + "/files/ws",
		"GET " + ws + "/lsp/ws",
		// The same live diagnostics stream, chat-scoped: one Broadcaster, one
		// StreamDef, two ways of naming a subscriber's scope (container.go
		// lspDef).
		"GET " + chat + "/lsp/ws",
		"GET " + chat + "/terminals/:sessionId/ws",
	}
}

// extraRoutes is the documented superset registered beyond the core spec:
// repo icon management, repo branch listing, project deletion (record-only
// cascade), and LSP document-sync notifications (04 §3, 10).
func extraRoutes() []string {
	const repo = "/v0/projects/:projectId/repos/:repoId"
	const ws = repo + "/workspaces/:wsId"
	const home = "/v0/projects/:projectId/home"
	// chat is the flat chat-scoped prefix (spec §7.1), same as specRoutes'.
	const chat = "/v0/chats/:chatId"
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
		// The same copy verb, chat-scoped, for the same reason the §2.4
		// chat-scoped block above lists the rest of the files surface.
		"POST " + chat + "/files/copy",
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
		// The same four review reads, chat-scoped (spec §4.2's SHARED bucket,
		// §8 step 4c) — see the specRoutes review chat-scoped comment for why
		// both prefixes are live.
		"GET " + chat + "/review/files",
		"GET " + chat + "/review/outline",
		"GET " + chat + "/review/patch",
		"GET " + chat + "/review/search",
		"POST " + ws + "/lsp/didOpen",
		"POST " + ws + "/lsp/didChange",
		"POST " + ws + "/lsp/didClose",
		// The same three document-sync notifications, chat-scoped — see the
		// specRoutes editor/LSP chat-scoped comment for why both prefixes are
		// live.
		"POST " + chat + "/lsp/didOpen",
		"POST " + chat + "/lsp/didChange",
		"POST " + chat + "/lsp/didClose",
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
		// The same identity read, chat-scoped (spec §4.2's SHARED bucket, §8
		// step 4c) — see the specRoutes review chat-scoped comment for why
		// both prefixes are live.
		"GET " + chat + "/identity",
		"DELETE " + ws + "/threads/:threadId",
		"PATCH " + ws + "/threads/:threadId/messages/:messageId",
		"DELETE " + ws + "/threads/:threadId/messages/:messageId",
		// Agentic-chat surface (00 agentic-engine spec §7, rescoped off the
		// workspace group onto the repo group by Task 17 of the 2026-08-28
		// sidebar backend plan: a chat's workspace is optional and mutable, so
		// no chat route names one any more): the REST + lifecycle WS the FE
		// Chats tab drives, nested under the repo group (chat.Register).
		"POST " + repo + "/chats",
		"GET " + repo + "/chats",
		"GET " + repo + "/chats/:id",
		"POST " + repo + "/chats/:id/switch",
		"POST " + repo + "/chats/:id/rename",
		"GET " + repo + "/chats/:id/handoff",
		"DELETE " + repo + "/chats/:id",
		// The chat's dry-run delete preview (backend addendum §1): the file
		// count a confirm dialog names, summed across every workspace the
		// subtree spans, before the caller commits to the delete above.
		"GET " + repo + "/chats/:id/delete-preview",
		// Chat placement: where a chat hangs in the Chats tree and where it sits
		// among its siblings. A route of its own rather than a field on the chat
		// PATCH-equivalents, because it writes something different in kind — a
		// chat's parent IS its context lineage, so this is what turns a standalone
		// chat into a THREAD of another and back.
		"PATCH " + repo + "/chats/:id/placement",
		// Promotion (model spec §4.2): a bubble fills its empty workspace slot,
		// keeping its id, title, placement and every turn it has taken. It is a
		// verb of its own rather than a field write because it cuts a branch,
		// adds a worktree and respawns the CLI in it — and it is one-way, since
		// a worktree is never demoted.
		"POST " + repo + "/chats/:id/promote",
		// The worktree LIFECYCLE verbs, on the thing actually being held
		// (chat-scoped API spec §4.3). Each resolves :id to the workspace behind
		// that chat's worktree and runs the very same handler body its
		// .../workspaces/:wsId twin below runs; the twins are retired in §8 step
		// 6b, once the frontend has moved.
		"POST " + repo + "/chats/:id/lock",
		"POST " + repo + "/chats/:id/sync",
		"POST " + repo + "/chats/:id/merge-into-parent",
		"POST " + repo + "/chats/:id/reparent",
		"POST " + repo + "/chats/:id/rebase-onto-parent",
		"POST " + repo + "/chats/:id/retry-provision",
		"POST " + repo + "/chats/:id/detach-holder",
		// The chat-keyed BRANCH rename: the half of PATCH .../workspaces/:wsId
		// that had no chat-scoped equivalent at all until now. It runs the very
		// same guards its :wsId twin does (locked branch, unprovisioned
		// placeholder, repo-wide name collision, adopted checkout) because it
		// routes through the same applyRename/hierarchy.RenameBranch body.
		//
		// Deliberately NOT folded into POST .../chats/:id/rename, which stays
		// title-only — see ChatRenameBranch for why that half of spec §7.5 was
		// declined.
		"PATCH " + repo + "/chats/:id/branch",
		// Batch branch import, relocated onto the surface that survives (spec §8
		// step 6b). It is a route of its own beside POST .../chats — that one
		// adopts ONE named branch, this one takes a set and resolves the repo's
		// open-PR graph across it, creating missing ancestors and falling back to
		// a placeholder for a branch held elsewhere. Its …/workspaces/import
		// mount above is still served by this very same handler.
		"POST " + repo + "/chats/import-batch",
		// Chat folder CRUD: the Chats panel's organisation layer. Repo-scoped,
		// and sharing ONE dense sibling space with the chats above — a folder and a
		// chat interleave at every level, which is why the placement route above and
		// these four are the two halves of the same gesture.
		"GET " + repo + "/chats/folders",
		"POST " + repo + "/chats/folders",
		"PATCH " + repo + "/chats/folders/:folderId",
		"DELETE " + repo + "/chats/folders/:folderId",
		"POST " + repo + "/chats/hooks",
		"GET " + repo + "/chats/providers",
		"GET " + repo + "/chats/ws",
		// The runner model added resume and never declared it here, which is exactly
		// the drift this audit exists to catch — it caught it. A chat whose CLI is gone
		// is not gone: its ledger and its provider conversation both outlive the
		// process, so a dormant chat can be handed a NEW runner that picks the
		// conversation back up.
		"POST " + repo + "/chats/:id/resume",
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
		"POST " + repo + "/chats/runners/:segid/mcp",
		//   stop: closing a chat TAB is not deleting the chat. The CLI is quit and
		//   the chat left DORMANT with its bound vendor conversation intact, which
		//   is exactly the state resume above exists to pick back up.
		"POST " + repo + "/chats/:id/stop",
		"POST " + repo + "/chats/:id/compact",
		// Two more ways a session on a chat can end without the chat itself
		// going anywhere: forcing the CLI into (or back out of) the host
		// terminal rather than the chat's own pane.
		"POST " + repo + "/chats/:id/switch-to-terminal",
		"POST " + repo + "/chats/:id/switch-to-native",
		// The read surface the Chats tab polls: the activity feed, a tool call's
		// full payload, open choices, message history, and provider telemetry.
		"GET " + repo + "/chats/:id/activity",
		"GET " + repo + "/chats/:id/activity/:toolId/payload",
		"GET " + repo + "/chats/:id/choices",
		"GET " + repo + "/chats/:id/messages",
		"GET " + repo + "/chats/:id/telemetry",
		// The provider's own slash-command list, so the composer can autocomplete
		// commands the CLI itself defines.
		"GET " + repo + "/chats/:id/slash-catalog",
		// The chat's sticky model/reasoning-effort choice, and its sticky
		// permission-level dial (each provider's own native mode).
		"PATCH " + repo + "/chats/:id/selection",
		"PUT " + repo + "/chats/:id/permission-level",
		// A human deciding a question the agent put to them mid-turn.
		"POST " + repo + "/chats/:id/choices/:choiceId/answer",
		// Submitting the user's own text into the chat.
		"POST " + repo + "/chats/:id/prompts",
		// The answer channel's other two legs (routes.go): the in-PTY relay parking
		// alive while the provider's gate stays open, and what it reports when the
		// provider decided at the terminal instead.
		"POST " + repo + "/chats/hooks/await",
		"POST " + repo + "/chats/hooks/abandon",
		// Provider PRIORITY + enable/disable is a GLOBAL user setting (the CLIs are
		// machine-level, not per workspace), so its write route mounts outside the
		// entity hierarchy beside /settings/terminal/profiles. It is the write
		// counterpart of the repo-scoped enriched GET .../chats/providers.
		"PUT /v0/settings/chat/providers",
		// The permission-level dial's own default, read and written the same way:
		// a machine-level setting, not a per-chat one, so a chat with no sticky
		// choice of its own falls back to this.
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
		// The home mount's own dry-run delete preview, for the same reason as
		// the repo block's above.
		"GET " + home + "/chats/:id/delete-preview",
		"POST " + home + "/chats/hooks",
		"GET " + home + "/chats/providers",
		"GET " + home + "/chats/ws",
		// The same two forced-terminal/native session moves, mounted here for
		// the same reason as the rest of this block: a home chat's session
		// behaves like any other chat's.
		"POST " + home + "/chats/:id/switch-to-terminal",
		"POST " + home + "/chats/:id/switch-to-native",
		// The repo home hosts chats like any workspace, so it gets the runner model's
		// resume too — same reason as the ws block above. A chat in the home is still a
		// chat: its CLI can die and be resumed.
		"POST " + home + "/chats/:id/resume",
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
		// Promotion re-mounted on the home group for the reason the whole block
		// is: TestHomeMountsEveryAgentRoute holds the two tables in step, and a
		// home chat is a chat.
		"POST " + home + "/chats/:id/promote",
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
		"PUT " + home + "/chats/:id/permission-level",
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
