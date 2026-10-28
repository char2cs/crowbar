package v0

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// withLSPLifecycle attaches the independent LSP subscription triggers to a
// StreamDef, scoping the refcount by scopeLSPOwnerID and delegating to the
// app-layer realtime service.
//
// This is the one lifecycle hook that does NOT reuse scopeWsID (T15): LSP is
// spec §4.2's OWNED bucket, so its session is per-CHAT, not per-workspace —
// editor's handlers key every engine call by scopeLSPOwnerID's same answer
// (handlers.Handlers.lspOwnerID), and the refcount that tears those sessions
// down on the last unsubscribe (LSPLifecycle.Shutdown → ReleaseWorkspace) has
// to match that key or the chat-scoped ones would never be released.
func withLSPLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeLSPOwnerID
	def.OnSubscribe = appContainer.Realtime.AcquireLSP
	def.OnUnsubscribe = appContainer.Realtime.ReleaseLSP
	return def
}

// withOriginSyncLifecycle attaches the protected-branch origin-sync trigger
// to a StreamDef, scoping by wsId resolved from the path or query and
// delegating to the app-layer realtime service. It CHAINS onto whatever
// OnSubscribe/OnUnsubscribe the StreamDef already carries (e.g.
// withWatcherLifecycle's watcher acquire) rather than replacing them, so both
// fire on every subscribe/unsubscribe.
func withOriginSyncLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	prevSubscribe := def.OnSubscribe
	prevUnsubscribe := def.OnUnsubscribe
	def.ScopeKey = scopeWsID
	def.OnSubscribe = func(scope string) {
		if prevSubscribe != nil {
			prevSubscribe(scope)
		}
		appContainer.Realtime.AcquireOriginSync(scope)
	}
	def.OnUnsubscribe = func(scope string) {
		if prevUnsubscribe != nil {
			prevUnsubscribe(scope)
		}
		appContainer.Realtime.ReleaseOriginSync(scope)
	}
	return def
}

// scopeWsID resolves the workspace id the per-scope WS resources (the file
// watcher, the protected-branch origin sync) are refcounted by for a client
// not bound to a chat: the :wsId path param, then the query. A chat-scoped
// client is refcounted by its binding instead (ws.StreamDef), which follows
// the chat across placement changes.
func scopeWsID(
	c *gin.Context,
) string {
	if id := c.Param("wsId"); id != "" {
		return id
	}
	return c.Query("wsId")
}

// withChatProviderPollLifecycle attaches the provider-poll subscription
// triggers to the agent-chat stream, scoped by the worktree the CHAT mount
// resolved and by nothing else.
//
// It keys on scopeChatWorktreeID rather than on scopeWsID, which the other
// lifecycle wrappers use, because scopeWsID also answers the :wsId PATH param —
// and the home mount (.../home/chats/ws) binds one. Keying on it would silently
// start a PR-status poll on the project-home row, which has no repo, no remote
// and no git surface at all: work that cannot succeed, on a stream that never
// asked for it. scopeChatWorktreeID reads ONLY the chat group's resolved
// worktree, so the repo mount and the home mount both resolve "" and no-op
// exactly as they did before this stream carried a lifecycle at all.
func withChatProviderPollLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeChatWorktreeID
	def.OnSubscribe = appContainer.Realtime.AcquireProviderPoll
	def.OnUnsubscribe = appContainer.Realtime.ReleaseProviderPoll
	return def
}

// scopeChatWorktreeID answers the workspace the /v0/chats/:chatId group's own
// resolveChatWorktree middleware put on the request, and "" anywhere that
// middleware did not run. It is deliberately NOT scopeWsID: see
// withChatProviderPollLifecycle.
func scopeChatWorktreeID(
	c *gin.Context,
) string {
	if ws, ok := reqscope.Workspace(c); ok {
		return ws.ID
	}
	return ""
}

// scopeLSPOwnerID resolves the key the LSP topic's lifecycle (withLSPLifecycle)
// refcounts by: the :chatId path param on /v0/chats/:chatId/lsp/ws, the only
// live mount of this stream (spec §8 step 6 retired the old
// /workspaces/:wsId/lsp/ws mount).
//
// LSP is spec §4.2's OWNED bucket, not shared like the file watcher/origin
// sync scopeWsID otherwise serves: keying by chat id rather than workspace id
// is what makes a chat-scoped subscriber refcount the same per-chat key its
// REST calls key their LSP session by (handlers.Handlers.lspOwnerID) — not
// the workspace those calls only resolve the worktree from.
func scopeLSPOwnerID(
	c *gin.Context,
) string {
	return c.Param("chatId")
}

// projectsDef serves the Projects topic. Its hierarchical namespace is the bare
// project id (spec §5). The snapshot returns every project as a wire DTO from
// the GORM store; the per-client prefix predicate filters it (spec §9).
func projectsDef(
	appContainer *app.Container,
) ws.StreamDef[dto.ProjectDTO] {
	return ws.StreamDef[dto.ProjectDTO]{
		Namespace: func(d dto.ProjectDTO) string { return d.ID },
		Serialize: func(d dto.ProjectDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  projectSnapshot(appContainer),
	}
}

// reposDef serves the Repos topic. Its hierarchical namespace is projectID/ID,
// so a project-scoped subscription ("p") receives every child repo (spec §5).
// The snapshot is project-scoped from the client's subscription prefix and reads
// the repos under that project from the GORM store (spec §9).
func reposDef(
	appContainer *app.Container,
) ws.StreamDef[dto.RepoDTO] {
	return ws.StreamDef[dto.RepoDTO]{
		Namespace: func(d dto.RepoDTO) string { return d.ProjectID + "/" + d.ID },
		Serialize: func(d dto.RepoDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  repoSnapshot(appContainer),
	}
}

// threadsDef serves the Threads topic. Its hierarchical namespace is
// projectID/repoID/workspaceID/ID, so a workspace-scoped subscription ("p/r/w")
// receives every thread in that workspace (spec §5); the per-client
// projectId/repoId/wsId Filters mirror the dual-served route's path params so
// path-first filter resolution scopes correctly. The snapshot is workspace-
// scoped from the client's subscription prefix and reads the workspace's threads
// from the global ReviewThread aggregate (W9: per-workspace thread storage is
// deferred — the aggregate stays on the global review_thread.db).
func threadsDef(
	appContainer *app.Container,
) ws.StreamDef[dto.ThreadDTO] {
	return ws.StreamDef[dto.ThreadDTO]{
		Namespace: func(d dto.ThreadDTO) string {
			return d.ProjectID + "/" + d.RepoID + "/" + d.WorkspaceID + "/" + d.ID
		},
		Serialize: func(d dto.ThreadDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  threadsSnapshot(appContainer),
		Filters: []ws.FilterDef[dto.ThreadDTO]{
			{Param: "projectId", Extract: func(d dto.ThreadDTO) string { return d.ProjectID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(d dto.ThreadDTO) string { return d.RepoID }, Match: ws.ExactMatch},
			{Param: "wsId", Extract: func(d dto.ThreadDTO) string { return d.WorkspaceID }, Match: ws.ExactMatch},
		},
	}
}

// terminalsDef serves the Terminal-session lifecycle topic, keyed by the chat
// that OWNS each session (spec §4.2's owned bucket, §7.4's straight re-key —
// no fan-out, because nothing here is shared).
//
// It is FlatNamespace with a single chatId Filter, the same shape git/files/lsp
// already use: the namespace is a bare chat id, not a hierarchical "p/r/w"
// path, so the hierarchical client-scope prefix must not be applied. The
// dual-served route is /v0/chats/:chatId/terminals, which binds no
// projectId/repoId/wsId at all — leaving the stream hierarchical would build an
// empty prefix that matches EVERY chat's frames, handing each subscriber every
// other chat's terminal lifecycle. The chatId Filter resolves from that path
// param and is what actually scopes a client.
//
// The snapshot derives from the in-memory engine registry (D6: no
// terminal_sessions view.db). The raw PTY byte stream is a separate,
// non-broadcast WebSocket.
func terminalsDef(
	appContainer *app.Container,
	engContainer *engine.Container,
) ws.StreamDef[dto.TerminalSessionDTO] {
	return ws.StreamDef[dto.TerminalSessionDTO]{
		Namespace:     func(d dto.TerminalSessionDTO) string { return d.ChatID },
		Serialize:     func(d dto.TerminalSessionDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:      terminalsSnapshot(appContainer, engContainer),
		FlatNamespace: true,
		Filters: []ws.FilterDef[dto.TerminalSessionDTO]{
			{Param: "chatId", Extract: func(d dto.TerminalSessionDTO) string { return d.ChatID }, Match: ws.ExactMatch},
		},
	}
}

// gitDef scopes the Git topic to the worktree a chat resolves to, bound per
// connection (chatWorkspaces) and Required: a client naming no chat gets
// nothing. The wire payload is the bare GitStatus.
func gitDef(
	appContainer *app.Container,
) ws.StreamDef[gitdomain.GitStatusEvent] {
	return ws.StreamDef[gitdomain.GitStatusEvent]{
		Namespace:     func(e gitdomain.GitStatusEvent) string { return e.WsID },
		Serialize:     func(e gitdomain.GitStatusEvent) ([]byte, error) { return json.Marshal(e.Status) },
		Snapshot:      gitSnapshot(appContainer),
		FlatNamespace: true,
		Filters: []ws.FilterDef[gitdomain.GitStatusEvent]{{
			Param:    "chatId",
			Extract:  func(e gitdomain.GitStatusEvent) string { return e.WsID },
			Match:    ws.ExactMatch,
			Required: true,
			Resolve:  chatWorkspaces(appContainer),
		}},
	}
}

// chatWorkspaces resolves bound chat ids to workspaces in one forest read. A
// failed read answers nothing, so every client keeps its current binding
// rather than going silent until the next placement change.
func chatWorkspaces(
	appContainer *app.Container,
) func(chatIDs []string) map[string]string {
	return func(chatIDs []string) map[string]string {
		if appContainer == nil || appContainer.Usecases == nil || appContainer.Usecases.Worktree == nil {
			return nil
		}
		answers, err := appContainer.Usecases.Worktree.WorkspacesForChats(context.Background(), chatIDs)
		if err != nil {
			slog.Warn("v0: resolve chat workspaces", "chats", len(chatIDs), "err", err)
			return nil
		}
		return answers
	}
}

// agentChatDef serves the agent-chat lifecycle event stream (GET
// .../repos/:repoId/chats/ws, and still GET .../home/chats/ws). It carries no
// snapshot: unlike the full-state resource streams above (projects, repos,
// threads, ...) a freshly-connected client simply waits for the next
// lifecycle event — there is no "current state" to replay.
//
// It has FOUR scoping filters and stays FlatNamespace, which is the shape the
// rows themselves force. The wsId Filter is what narrows the HOME mount, whose
// RequireHomeWorkspace injects a :wsId for it to resolve; it goes inactive at
// the repo mount, which binds no :wsId at all — and that inactive filter used
// to be the whole of this stream's scoping, so a repo-scoped client received
// every OTHER repo's chat events too. The repoId Filter closes that: the repo
// mount binds :repoId, and every chat frame carries the repo its row actually
// runs in (see agent_chat_scope.go).
//
// The projectId Filter closes what repoId structurally CANNOT. Both live mounts
// nest under /projects/:projectId, so both resolve it; the flat
// /v0/chats/:chatId/ws mount binds neither it nor :repoId and is scoped by its
// own chatId Filter instead, so neither is Required.
//
// The stream is NOT given the hierarchical projectId/repoId/wsId namespace
// threadsDef and terminalsDef use, because half the rows on this feed cannot
// fill one. A FOLDER carries no workspace and no repo id of its own, neither
// does a bubble at the panel root, and neither does ANY row in a project home —
// that workspace owns no repo; under a hierarchical namespace those frames
// would resolve "//" and be dropped from every repo-scoped subscriber, silently
// killing the live folder feed. matchScopeOrUnscoped is what lets both kinds
// coexist on one stream: a frame that KNOWS a scope is held to it, and one that
// cannot know it reaches everyone.
//
// That escape hatch is why the two filters are needed rather than one. It is
// keyed per FIELD, so a repo-less frame skipped the repoId filter entirely —
// and a project home's chats are all repo-less, so a home chat's streamed text
// in one project was delivered to repo-scoped sockets in EVERY other project in
// the process. Answering the project as well bounds them: repo-less still means
// "every repo", but now only every repo OF THIS PROJECT, which is the narrowest
// scope that keeps the folder rows and root bubbles this hatch exists for. A
// frame that resolves neither field still reaches everyone — the same disclosed
// limitation ChatTreeUsecase.ListInRepo already carries for folders, unchanged,
// rather than a new silent drop.
func agentChatDef() ws.StreamDef[dto.AgentChatEvent] {
	return ws.StreamDef[dto.AgentChatEvent]{
		Namespace:     func(e dto.AgentChatEvent) string { return e.WorkspaceID },
		Serialize:     func(e dto.AgentChatEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:      func(string) []dto.AgentChatEvent { return nil },
		FlatNamespace: true,
		Filters: []ws.FilterDef[dto.AgentChatEvent]{
			{Param: "wsId", Extract: func(e dto.AgentChatEvent) string { return e.WorkspaceID }, Match: ws.ExactMatch},
			{Param: "projectId", Extract: func(e dto.AgentChatEvent) string { return e.ProjectID }, Match: matchScopeOrUnscoped},
			{Param: "repoId", Extract: func(e dto.AgentChatEvent) string { return e.RepoID }, Match: matchScopeOrUnscoped},
			{Param: "chatId", Extract: func(e dto.AgentChatEvent) string { return e.ChatID }, Match: ws.ExactMatch},
		},
		// message_delta is the one kind on this feed that is already "the
		// full state so far" by construction (see
		// hub.BroadcastAgentChatMessageDelta's own doc comment) — a client
		// that misses several is exactly as correct as one that saw every
		// one, as long as it eventually sees the latest. A fast-streaming
		// provider can emit these far more often than this feed's other,
		// genuinely stateful kinds (turn_started, folder events, ...), and
		// those must keep the ordinary bounded-queue-then-disconnect
		// contract untouched — CoalesceKey is what lets the two coexist on
		// one broadcaster. Keyed by (chat, stream kind, message id) so two chats,
		// two co-open items in one chat's turn, or the answer and the thinking
		// that runs alongside it never coalesce into each other. The kind is in
		// the key structurally rather than because the two id spaces happen to
		// differ today: collapsing a thought onto an answer would replace the
		// reply with the reasoning that preceded it.
		CoalesceKey: func(e dto.AgentChatEvent) (string, bool) {
			if e.Kind != dto.AgentChatKindMessageDelta || e.Message == nil {
				return "", false
			}
			return e.ChatID + "\x00" + e.Message.Kind + "\x00" + e.Message.ID, true
		},
	}
}

// matchScopeOrUnscoped holds a frame that KNOWS one of its scoping ids to
// exactly that id, and lets one that does not reach every subscriber. It serves
// both of agentChatDef's scoping Filters, projectId and repoId.
//
// The second half is not laxness, it is the honest answer for the rows that
// have no such id to be held to: a folder row carries no repo, neither does a
// bubble whose ancestry owns no workspace, and neither does any row in a
// project home. Refusing those would drop the live folder feed the Chats panel
// repaints from. ws.ExactMatch would do exactly that, which is why this is its
// own function and not that one.
//
// It is also why ONE of these filters is not enough. The hatch opens per FIELD,
// so a repo-less frame was held to nothing at all — it took a second field the
// same row CAN answer to bound it. See agentChatDef.
func matchScopeOrUnscoped(
	param string,
	value string,
) bool {
	return value == "" || param == value
}

// lspDef scopes the LSP diagnostics topic to a single owned session, named by
// the chat that owns it.
//
// The chatId filter scopes a client to exactly the diagnostics that chat's
// own lsp/didOpen etc. calls produced (handlers.Handlers.lspOwnerID) — never a
// sibling chat's, even one sharing this chat's worktree. Unlike gitDef/filesDef
// it is not bound to a workspace: editor/LSP is spec §4.2's OWNED bucket, so
// an event has exactly one owner.
//
// The old /workspaces/:wsId/lsp/ws mount that once needed a second, matching
// wsId filter beside this one is gone (spec §8 step 6) — /chats/:chatId/lsp/ws
// is the only live mount of this broadcaster's Handle (router.go).
func lspDef(
	appContainer *app.Container,
	engContainer *engine.Container,
) ws.StreamDef[lspdomain.DiagnosticsEvent] {
	return ws.StreamDef[lspdomain.DiagnosticsEvent]{
		Namespace:     func(e lspdomain.DiagnosticsEvent) string { return e.WsID },
		Serialize:     func(e lspdomain.DiagnosticsEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:      lspSnapshot(appContainer, engContainer),
		FlatNamespace: true,
		Filters: []ws.FilterDef[lspdomain.DiagnosticsEvent]{
			{Param: "chatId", Extract: func(e lspdomain.DiagnosticsEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}
