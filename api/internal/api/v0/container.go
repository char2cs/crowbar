package v0

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the v0 delivery surface: the seven realtime topics plus REST
// routes. It implements hub.Subscriber so app-layer broadcasts reach connected
// clients.
//
// The push-only Broadcaster[T] instances held here are workspaces, git, files,
// and lsp. The Terminal topic is intentionally NOT a Broadcaster[T]: PTY streams
// are bidirectional, so the Terminal topic is served by the engine.Attach
// WebSocket handler (endpoints/terminal/handlers/ws.go), whose ring-buffer
// replay is its snapshot-on-subscribe (03 §1a). It is wired separately in
// router.go.
type Container struct {
	projects   *ws.Broadcaster[dto.ProjectDTO]
	repos      *ws.Broadcaster[dto.RepoDTO]
	workspaces *ws.Broadcaster[dto.WorkspaceDTO]
	threads    *ws.Broadcaster[dto.ThreadDTO]
	terminals  *ws.Broadcaster[dto.TerminalSessionDTO]
	git        *ws.Broadcaster[gitdomain.GitStatusEvent]
	files      *ws.Broadcaster[domain.FileChangeEvent]
	lsp        *ws.Broadcaster[lspdomain.DiagnosticsEvent]
	agentChats *ws.Broadcaster[dto.AgentChatEvent]
	// chatScopes answers what each agent-chat frame namespaces under — see
	// agent_chat_scope.go.
	chatScopes *agentChatScopes
	app        *app.Container
	eng        *engine.Container
}

var _ hub.Subscriber = (*Container)(nil)

// New builds the v0 container and registers it as a hub subscriber.
//
// The lazy WS resource lifecycles (03 §6) are owned by the app-layer realtime
// service: the Files∪Git topics' OnSubscribe/OnUnsubscribe hooks drive the
// per-workspace file watcher via AcquireWatcher/ReleaseWatcher, and the LSP
// topic's hooks drive the per-workspace LSP host via AcquireLSP/ReleaseLSP. This
// container only wires the subscription triggers (transport); the watcher and
// LSP goroutines, the working-tree sync command, and the hub fan-out all live
// in app, and app.Container.Close tears every live resource down on shutdown.
func New(
	appContainer *app.Container,
	engContainer *engine.Container,
) *Container {
	if appContainer == nil {
		panic("v0: appContainer is required")
	}
	c := &Container{
		projects:   ws.NewBroadcaster(projectsDef(appContainer)),
		repos:      ws.NewBroadcaster(reposDef(appContainer)),
		workspaces: ws.NewBroadcaster(withProviderPollLifecycle(workspacesDef(appContainer), appContainer)),
		threads:    ws.NewBroadcaster(threadsDef(appContainer)),
		terminals:  ws.NewBroadcaster(terminalsDef(appContainer, engContainer)),
		git:        ws.NewBroadcaster(withOriginSyncLifecycle(withWatcherLifecycle(gitDef(appContainer), appContainer), appContainer)),
		files:      ws.NewBroadcaster(withWatcherLifecycle(filesDef(), appContainer)),
		lsp:        ws.NewBroadcaster(withLSPLifecycle(lspDef(appContainer, engContainer), appContainer)),
		agentChats: ws.NewBroadcaster(withChatProviderPollLifecycle(agentChatDef(), appContainer)),
		chatScopes: newAgentChatScopes(),
		app:        appContainer,
		eng:        engContainer,
	}
	if engContainer != nil && engContainer.LSP != nil {
		engContainer.LSP.OnDiagnostics(c.lsp.Push)
	}
	if engContainer != nil && engContainer.Terminal != nil {
		engContainer.Terminal.OnSessionEnded(c.onTerminalEnded)
		engContainer.Terminal.OnSessionState(c.onTerminalState)
		if appContainer.Usecases != nil && appContainer.Usecases.TerminalMeta != nil {
			engContainer.Terminal.SetMetaStore(appContainer.Usecases.TerminalMeta)
		}
	}
	appContainer.Hub.Register(c)
	return c
}

// onTerminalEnded emits an "ended" lifecycle frame when a PTY exits on its own
// (the reap path in the terminal engine). The handler-driven Kill path also
// pushes an "ended" frame; the broadcaster's idempotent full-replace makes the
// duplicate harmless. exitCode is the process exit code; it is included in the
// frame when >=0 (known).
//
// The frame namespaces under the OWNING CHAT alone. It no longer resolves a
// project/repo pair on the way out: terminal is spec §4.2's owned bucket, so
// the topic is a single chat and there is nothing to fan out to (§7.4). That
// also removes a silent failure mode — the old lookup returned ("", "") for an
// unresolvable workspace and published the frame under the namespace "//",
// where no subscriber was listening.
func (c *Container) onTerminalEnded(
	_ context.Context,
	chatID string,
	sessionID string,
	exitCode int,
) {
	endedAt := time.Now().UTC()
	ended := dto.TerminalSessionDTOFrom(
		sessionID,
		chatID,
		"",
		"ended",
		endedAt,
	)
	ended.EndedAt = &endedAt
	if exitCode >= 0 {
		ended.ExitCode = &exitCode
	}
	c.terminals.Push(ended)
}

// onTerminalState emits a lifecycle frame when a session transitions to
// "detached" or "suspended", namespaced under the chat that owns the session.
func (c *Container) onTerminalState(
	_ context.Context,
	chatID string,
	sessionID string,
	state string,
) {
	d := dto.TerminalSessionDTOFrom(
		sessionID,
		chatID,
		"",
		state,
		time.Now().UTC(),
	)
	c.terminals.Push(d)
}

// resolveWorkspaceScope returns the project and repo ids owning workspaceID,
// or empty strings when the workspace cannot be resolved.
func (c *Container) resolveWorkspaceScope(
	ctx context.Context,
	workspaceID string,
) (string, string) {
	if c.app == nil {
		return "", ""
	}
	ws, err := c.app.Repositories.Workspace.Get(ctx, workspaceID)
	if err != nil {
		return "", ""
	}
	return ws.ProjectID, ws.RepoID
}

// withWatcherLifecycle attaches the Files∪Git watcher subscription triggers to a
// StreamDef, scoping the refcount by wsId resolved from the path or query and
// delegating to the app-layer realtime service.
func withWatcherLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeWsID
	def.OnSubscribe = appContainer.Realtime.AcquireWatcher
	def.OnUnsubscribe = appContainer.Realtime.ReleaseWatcher
	return def
}

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

// withProviderPollLifecycle attaches the per-active-WS-connection provider-poll
// subscription triggers to a StreamDef, scoping the refcount by wsId resolved
// from the path or query and delegating to the app-layer realtime service
// (D10/§11). Only the single-workspace (:wsId) subscription carries a wsId; the
// workspace list scope (.../workspaces, no :wsId) resolves to "" and the
// manager no-ops, so the poll starts only when a client watches one workspace.
func withProviderPollLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeWsID
	def.OnSubscribe = appContainer.Realtime.AcquireProviderPoll
	def.OnUnsubscribe = appContainer.Realtime.ReleaseProviderPoll
	return def
}

// scopeWsID resolves the workspace id the per-scope WS resources (the file
// watcher, the LSP host, the protected-branch origin sync) are refcounted by:
// the path param, then the chat group's already-resolved workspace, then the
// query param, mirroring the dual-served Git/Files/LSP routes (T15).
//
// The reqscope step is what keeps those resources alive for a CHAT-scoped
// subscriber. A client on /v0/chats/:chatId/git/status binds no :wsId at all,
// so without it this resolved "" — and the acquire/release calls this feeds are
// per-workspace refcounts, so the file watcher that PRODUCES git-status pushes
// would never have been started for that workspace. The subscription would
// connect, replay its snapshot, and then sit silent forever, which is the
// failure mode the whole re-key would otherwise have shipped. resolveChatWorktree
// has already resolved the workspace by the time the broadcaster reads the
// scope, so this is a context read, not a second resolve.
func scopeWsID(
	c *gin.Context,
) string {
	if id := c.Param("wsId"); id != "" {
		return id
	}
	if ws, ok := reqscope.Workspace(c); ok && ws.ID != "" {
		return ws.ID
	}
	return c.Query("wsId")
}

// withChatProviderPollLifecycle attaches the provider-poll subscription
// triggers to the agent-chat stream, scoped by the worktree the CHAT mount
// resolved and by nothing else.
//
// It exists rather than reusing withProviderPollLifecycle because that one
// keys on scopeWsID, which also answers the :wsId PATH param — and the home
// mount (.../home/chats/ws) binds one. Reusing it would silently start a
// PR-status poll on the project-home row, which has no repo, no remote and no
// git surface at all: work that cannot succeed, on a stream that never asked
// for it. scopeChatWorktreeID reads ONLY the chat group's resolved worktree, so
// the repo mount and the home mount both resolve "" and no-op exactly as they
// did before this stream carried a lifecycle at all.
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
// refcounts by: the chat id on the new /v0/chats/:chatId/lsp/ws mount, or
// scopeWsID's answer (the workspace id) on every other mount.
//
// LSP is spec §4.2's OWNED bucket, not shared like the file watcher/origin
// sync scopeWsID otherwise serves: a chat id is checked FIRST, ahead of
// scopeWsID's own reqscope fallback, so that a chat-scoped subscriber
// refcounts the same per-chat key its REST calls key their LSP session by
// (handlers.Handlers.lspOwnerID) — not the workspace those calls only resolve
// the worktree from.
func scopeLSPOwnerID(
	c *gin.Context,
) string {
	if id := c.Param("chatId"); id != "" {
		return id
	}
	return scopeWsID(c)
}

// PushProject implements hub.Subscriber.
func (c *Container) PushProject(
	p dto.ProjectDTO,
) {
	c.projects.Push(p)
}

// PushRepo implements hub.Subscriber.
func (c *Container) PushRepo(
	r dto.RepoDTO,
) {
	c.repos.Push(r)
}

// PushWorkspace implements hub.Subscriber. It serves BOTH scoping answers from
// one push — the workspace topic a workspace-scoped client watches, and the
// chat topic a chat-scoped one does (spec §5) — which is the same shape PushGit
// already takes for the same reason: the two are answers to one question, and a
// client that got them from independently-built frames would watch its sidebar
// disagree with itself.
func (c *Container) PushWorkspace(
	w dto.WorkspaceDTO,
) {
	c.workspaces.Push(w)
	c.pushChatWorktree(w)
}

// pushChatWorktree fans a workspace's git state out on the CHAT feed, keyed on
// the chat that owns it.
//
// It reads the owning chat id off the frame rather than resolving one, and that
// is deliberate: enrichFrame has already resolved it, through the same
// branch-preferring ResolveOwningChat every other surface uses, so taking it
// here means the chat frame names exactly the row the workspace frame says owns
// this worktree — never a second, independently derived answer that could pick
// a different one.
//
// A workspace with no resolved owning chat pushes nothing. That is the honest
// answer rather than a broadcast to nobody: such a row is the orphan spec §0
// diagnosed, it has no chat for a client to draw it on, and the boot backfill
// (not this push) is what gives it one.
//
// RepoID comes straight off the workspace, with no chat-forest walk: unlike a
// bubble, whose repo is derived from where its cwd lands, a worktree-owning row
// names its own repo outright.
func (c *Container) pushChatWorktree(
	w dto.WorkspaceDTO,
) {
	if w.OwningChatID == "" {
		return
	}
	// Working is deliberately LEFT UNSET. The frame's own Working field is the
	// CHAT's folded turn state (see AgentChatEvent.Working), and a workspace's
	// is a different fact — a long-running git operation, not a conversation in
	// flight. Putting the workspace's answer there would make a client's spinner
	// follow whichever of the two moved last. The workspace's own busy state
	// rides the worktree object, where it belongs.
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:      w.OwningChatID,
		WorkspaceID: w.ID,
		RepoID:      w.RepoID,
		Kind:        dto.AgentChatKindWorktreeState,
		Worktree:    dto.ChatWorktreeFrom(w),
	})
}

// PushThread implements hub.Subscriber.
func (c *Container) PushThread(
	t dto.ThreadDTO,
) {
	c.threads.Push(t)
}

// PushTerminalSession implements hub.Subscriber.
func (c *Container) PushTerminalSession(
	s dto.TerminalSessionDTO,
) {
	c.terminals.Push(s)
}

// PushGit implements hub.Subscriber. It wraps the status in an event carrying
// BOTH scoping answers — the workspace it describes, and every chat currently
// holding that workspace — so one Push serves the workspace-scoped route and
// fans out to the chat-scoped one in a single pass (spec §7.4).
//
// This is the single production push site for the git topic: hub.BroadcastGit
// is called only by the realtime watcher dispatcher's OnGitStatus. The "watcher
// broadcast" that git's own write handlers name in their doc comments IS this
// path — a write completes, the file watcher notices, and the post-op state
// arrives here — not a second, handler-driven push of its own.
//
// The set is resolved at PUSH time rather than at connect time on purpose: a
// client's predicate is compiled once, when it subscribes, so a chat forked
// onto this worktree AFTER that moment can only be reached by news the event
// itself carries. The resolve costs one chat-forest read per push, and the
// watcher already dedups against its previous status, so it runs on real
// change rather than per tick.
func (c *Container) PushGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	// A clean tree carries a nil Files slice; normalise so the frame
	// serialises with files: [] (never null), matching the REST DTO.
	if status.Files == nil {
		status.Files = []gitdomain.GitFile{}
	}
	c.git.Push(gitdomain.GitStatusEvent{
		WsID:    wsID,
		ChatIDs: c.chatsHolding(context.Background(), wsID),
		Status:  status,
	})
}

// chatsHolding answers which chats currently resolve to workspaceID, degrading
// to the empty set rather than an error: a fan-out that cannot be resolved
// reaches nobody, which leaves the workspace-scoped subscribers on the same
// event untouched. It never returns a nil-vs-empty distinction, because a set
// carrying nothing already matches nobody (ws.FilterDef.ExtractSet).
func (c *Container) chatsHolding(
	ctx context.Context,
	workspaceID string,
) []string {
	if c.app == nil || c.app.Usecases == nil || c.app.Usecases.Worktree == nil {
		return nil
	}
	chatIDs, err := c.app.Usecases.Worktree.ChatsForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil
	}
	return chatIDs
}

// PushFile implements hub.Subscriber. It stamps the event with every chat
// currently holding the workspace it describes, so one Push serves the
// workspace-scoped and home routes and fans out to the chat-scoped one in a
// single pass (spec §7.4) — the same shape PushGit takes, for the same reason.
//
// The set is resolved at PUSH time rather than at connect time on purpose: a
// client's predicate is compiled once, when it subscribes, so a chat forked
// onto this worktree AFTER that moment can only be reached by news the event
// itself carries.
//
// Unlike git's, this push arrives per DEBOUNCED filesystem event rather than
// only on a changed status, so the resolve rides a burst rather than a real
// change — it is one chat-forest read against the same in-memory rows the
// resolver already serves, and the alternative (resolving at connect) cannot
// answer the fork-after-connect case at all.
func (c *Container) PushFile(
	evt domain.FileChangeEvent,
) {
	evt.ChatIDs = c.chatsHolding(context.Background(), evt.WsID)
	c.files.Push(evt)
}

// PushAgentChat implements hub.Subscriber. It fans an agent-chat lifecycle
// event out to every subscriber of the agent-chat WebSocket (GET
// .../repos/:repoId/chats/ws, and .../home/chats/ws) whose scope the frame's
// namespace falls under.
//
// It takes the FRESH scope: every kind reaching this method is structural — a
// create, a placement, a workspace slot filled, a delete — and each is exactly
// the kind of change that can move a bubble into another repo, so the memo the
// streaming frames read must not survive it.
func (c *Container) PushAgentChat(
	chatID string,
	workspaceID string,
	kind string,
	working bool,
) {
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		RepoID:      c.freshAgentChatRepo(chatID, workspaceID),
		Kind:        kind,
		Working:     working,
	})
}

// PushAgentChatTerminalWait implements hub.Subscriber. It fans the terminal-wait
// edge out on the SAME workspace-scoped agent-chat WebSocket as PushAgentChat: it
// is a fact about a conversation, so it belongs on the conversation feed rather
// than on a second socket that would have to be kept in order with it.
func (c *Container) PushAgentChatTerminalWait(
	chatID string,
	workspaceID string,
	wait *dto.AgentTerminalWaitDTO,
) {
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:       chatID,
		WorkspaceID:  workspaceID,
		RepoID:       c.agentChatRepo(chatID, workspaceID),
		Kind:         dto.AgentChatKindTerminalWait,
		TerminalWait: wait,
	})
}

// PushAgentChatPromptSettled implements hub.Subscriber, on the SAME
// workspace-scoped agent-chat WebSocket as every other fact about a conversation.
func (c *Container) PushAgentChatPromptSettled(
	chatID string,
	workspaceID string,
	requestID string,
) {
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:          chatID,
		WorkspaceID:     workspaceID,
		RepoID:          c.agentChatRepo(chatID, workspaceID),
		Kind:            dto.AgentChatKindPromptSettled,
		ClientRequestID: requestID,
	})
}

// PushAgentChatMessageDelta implements hub.Subscriber, on the SAME
// workspace-scoped agent-chat WebSocket as every other conversation fact.
func (c *Container) PushAgentChatMessageDelta(
	chatID string,
	workspaceID string,
	messageID string,
	text string,
) {
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		RepoID:      c.agentChatRepo(chatID, workspaceID),
		Kind:        dto.AgentChatKindMessageDelta,
		Message:     &dto.AgentStreamingMessageDTO{ID: messageID, Text: text},
	})
}

// PushAgentChatFolder implements hub.Subscriber. It fans a chat-folder lifecycle
// event (folder_created/folder_updated/folder_deleted) out on the SAME
// workspace-scoped agent-chat WebSocket as PushAgentChat — one feed for "what
// changed about this workspace's chats panel", whether the row that changed was
// a chat or one of the folders it shares a sibling space with. Two feeds would
// have to be kept in order with each other, and one gesture writes both kinds.
//
// The frame carries the folder id and no row, which is what the stream's own
// shape requires: it has no snapshot, so a client reads folders over REST and a
// frame here means "read them again".
//
// A folder at the panel root carries no workspace and no repo id, so its frame
// resolves the EMPTY repo and reaches every subscriber (matchRepoOrUnscoped) —
// the folder half of the repo boundary is the disclosed limitation
// ChatTreeUsecase.ListInRepo already carries, unchanged here. The fresh
// resolution is taken anyway for its other half: a folder move is a structural
// change that can have carried bubbles into another repo with it, and the memo
// those bubbles' streaming frames read must not survive it.
func (c *Container) PushAgentChatFolder(
	folderID string,
	workspaceID string,
	kind string,
) {
	c.agentChats.Push(dto.AgentChatEvent{
		FolderID:    folderID,
		WorkspaceID: workspaceID,
		RepoID:      c.freshAgentChatRepo(folderID, workspaceID),
		Kind:        kind,
	})
}

// PushAgentRunner implements hub.Subscriber. It fans a runner lifecycle event
// (started/session_bound/moved/exited) out on the SAME workspace-scoped
// agent-chat WebSocket as PushAgentChat (GET .../workspaces/:wsId/chats/ws)
// — one feed for "what changed about this workspace's agent chats", whether the
// change came from the chat aggregate or from the runner pointed at it. A second
// socket would buy nothing and would have to be kept in order with the first.
//
// The frame is the same wire type, with RunnerID set (it is empty on the chat
// kinds). Its ChatID is the chat the runner is pointed at AS OF this event, so a
// `moved` frame tells the client which chat the CLI moved INTO and which tab must
// follow it. agentChatDef's wsId Filter does the scoping, exactly as for
// PushAgentChat; this method pushes unconditionally.
func (c *Container) PushAgentRunner(
	runnerID string,
	workspaceID string,
	chatID string,
	kind string,
) {
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID: chatID, WorkspaceID: workspaceID,
		RepoID: c.freshAgentChatRepo(chatID, workspaceID),
		Kind:   kind, RunnerID: runnerID,
	})
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

// workspacesDef serves the Workspaces topic. Its hierarchical namespace is
// projectID/repoID/ID, so a repo-scoped subscription ("p/r") receives every
// child workspace (spec §5). The snapshot is repo-scoped from the client's
// subscription prefix and carries the merge-eligibility overlay (spec §9/§10).
func workspacesDef(
	appContainer *app.Container,
) ws.StreamDef[dto.WorkspaceDTO] {
	return ws.StreamDef[dto.WorkspaceDTO]{
		Namespace: func(d dto.WorkspaceDTO) string {
			return d.ProjectID + "/" + d.RepoID + "/" + d.ID
		},
		Serialize: func(d dto.WorkspaceDTO) ([]byte, error) { return json.Marshal(d) },
		Snapshot:  workspacesSnapshot(appContainer),
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

// gitDef scopes the Git topic to a single worktree, named either way its two
// live routes name one. The wire payload is a bare GitStatus (the embedded
// Status), matching the REST snapshot of the dual-serve route; neither scoping
// field is ever serialized onto the Git stream.
//
// ONE StreamDef serves both routes, because there is one Broadcaster: it is
// built once, in New, and every client registers against the same compiled
// def regardless of which route it upgraded on. So the two filters below are
// not alternatives the wiring picks between — both are declared for every
// client, and each client activates whichever one its own request resolves:
//
//   - /projects/:p/repos/:r/workspaces/:wsId/git/status binds :wsId and no
//     :chatId, so the wsId filter is active and scopes it to exactly one
//     workspace, exactly as before this step, and the chatId filter resolves
//     nothing and goes inactive.
//   - /chats/:chatId/git/status binds :chatId and no :wsId, so the chatId
//     filter is active and matches by MEMBERSHIP against the fan-out set the
//     event carries — every chat holding that worktree, resolved at push time
//     (PushGit) — while the wsId filter goes inactive.
//
// matchesAll requires every ACTIVE filter to match, so each client is scoped by
// the one it actually resolved, and neither route can see the other's traffic.
//
// NEITHER filter is Required, and that is forced rather than chosen. Required
// makes a client that resolves no value for the param match NOTHING instead of
// dropping the filter — the guard ws.ChatFanoutFilter carries, and the right
// one once /chats is the only way in. Setting it on chatId today would refuse
// every client of the still-live workspace-scoped route, which cannot resolve a
// :chatId; setting it on wsId would refuse every chat-scoped one. The trap
// Required exists to close — a client resolving NEITHER param and being handed
// every workspace on the daemon — needs a mount binding neither, and the two
// above are the only mounts of this broadcaster's Handle (router.go). When
// spec §8 step 6 deletes the workspace-scoped mount, the wsId filter goes with
// it and the chatId filter becomes ws.ChatFanoutFilter, Required and all.
func gitDef(
	appContainer *app.Container,
) ws.StreamDef[gitdomain.GitStatusEvent] {
	return ws.StreamDef[gitdomain.GitStatusEvent]{
		Namespace:     func(e gitdomain.GitStatusEvent) string { return e.WsID },
		Serialize:     func(e gitdomain.GitStatusEvent) ([]byte, error) { return json.Marshal(e.Status) },
		Snapshot:      gitSnapshot(appContainer),
		FlatNamespace: true,
		Filters: []ws.FilterDef[gitdomain.GitStatusEvent]{
			{Param: "wsId", Extract: func(e gitdomain.GitStatusEvent) string { return e.WsID }, Match: ws.ExactMatch},
			{
				Param:      "chatId",
				ExtractSet: func(e gitdomain.GitStatusEvent) []string { return e.ChatIDs },
				Match:      ws.ExactMatch,
			},
		},
	}
}

// filesDef scopes the Files topic to a single worktree, named either way its
// live routes name one. Unlike gitDef the whole event goes on the wire, so the
// fan-out set is the one field held back by its json tag rather than by the
// Serialize lambda.
//
// It carries NO Snapshot, and that is the shape of the topic rather than an
// omission: a file-change event is news, not state. A connecting client has
// already fetched the tree over REST and has nothing to replay — which is why
// this step needs no chat-scoped snapshot resolver of the kind gitSnapshot grew
// (snapshots.go), and why a chat-scoped subscriber's first frame is simply the
// next change.
//
// ONE StreamDef serves every mount, because there is one Broadcaster: it is
// built once, in New, and every client registers against the same compiled def
// regardless of which route it upgraded on. So the two filters below are not
// alternatives the wiring picks between — both are declared for every client,
// and each client activates whichever one its own request resolves:
//
//   - /projects/:p/repos/:r/workspaces/:wsId/files/ws binds :wsId, so the wsId
//     filter is active and scopes it to exactly one workspace, exactly as
//     before this step.
//   - /projects/:p/home/files/ws binds no :wsId in its PATH, but
//     RequireHomeWorkspace injects one before the upgrade runs, so it resolves
//     the same wsId filter and is likewise untouched.
//   - /chats/:chatId/files/ws binds :chatId and no :wsId, so the chatId filter
//     is active and matches by MEMBERSHIP against the fan-out set the event
//     carries — every chat holding that worktree, resolved at push time
//     (PushFile) — while the wsId filter goes inactive.
//
// matchesAll requires every ACTIVE filter to match, so each client is scoped by
// the one it actually resolved, and no mount can see another's traffic.
//
// NEITHER filter is Required, forced by the same argument gitDef records and
// with one more mount to satisfy: Required on chatId would refuse every client
// of the workspace-scoped route AND every client of the home route, neither of
// which can resolve a :chatId; Required on wsId would refuse every chat-scoped
// one. The trap Required exists to close — a client resolving NEITHER param and
// being handed every workspace on the daemon — needs a mount binding neither,
// and the three above are the only mounts of this broadcaster's Handle
// (router.go, home/routes.go).
func filesDef() ws.StreamDef[domain.FileChangeEvent] {
	return ws.StreamDef[domain.FileChangeEvent]{
		Namespace:     func(e domain.FileChangeEvent) string { return e.WsID },
		Serialize:     func(e domain.FileChangeEvent) ([]byte, error) { return json.Marshal(e) },
		FlatNamespace: true,
		Filters: []ws.FilterDef[domain.FileChangeEvent]{
			{Param: "wsId", Extract: func(e domain.FileChangeEvent) string { return e.WsID }, Match: ws.ExactMatch},
			{
				Param:      "chatId",
				ExtractSet: func(e domain.FileChangeEvent) []string { return e.ChatIDs },
				Match:      ws.ExactMatch,
			},
		},
	}
}

// agentChatDef serves the agent-chat lifecycle event stream (GET
// .../repos/:repoId/chats/ws, and still GET .../home/chats/ws). It carries no
// snapshot: unlike the full-state resource streams above (projects, repos,
// workspaces, ...) a freshly-connected client simply waits for the next
// lifecycle event — there is no "current state" to replay.
//
// It has TWO scoping filters and stays FlatNamespace, which is the shape the
// rows themselves force. The wsId Filter is what narrows the HOME mount, whose
// RequireHomeWorkspace injects a :wsId for it to resolve; it goes inactive at
// the repo mount, which binds no :wsId at all — and that inactive filter used
// to be the whole of this stream's scoping, so a repo-scoped client received
// every OTHER repo's chat events too. The repoId Filter closes that: the repo
// mount binds :repoId, and every chat frame carries the repo its row actually
// runs in (see agent_chat_scope.go).
//
// The stream is NOT given the hierarchical projectId/repoId/wsId namespace
// threadsDef and terminalsDef use, because half the rows on this feed cannot
// fill one. A FOLDER carries no workspace and no repo id of its own, and
// neither does a bubble at the panel root; under a hierarchical namespace those
// frames would resolve "//" and be dropped from every repo-scoped subscriber,
// silently killing the live folder feed. matchRepoOrUnscoped is what lets both
// kinds coexist on one stream: a frame that KNOWS its repo is held to it, and
// one that cannot know it reaches everyone — the same disclosed limitation
// ChatTreeUsecase.ListInRepo already carries for folders, unchanged, rather
// than a new silent drop.
func agentChatDef() ws.StreamDef[dto.AgentChatEvent] {
	return ws.StreamDef[dto.AgentChatEvent]{
		Namespace:     func(e dto.AgentChatEvent) string { return e.WorkspaceID },
		Serialize:     func(e dto.AgentChatEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:      func(string) []dto.AgentChatEvent { return nil },
		FlatNamespace: true,
		Filters: []ws.FilterDef[dto.AgentChatEvent]{
			{Param: "wsId", Extract: func(e dto.AgentChatEvent) string { return e.WorkspaceID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(e dto.AgentChatEvent) string { return e.RepoID }, Match: matchRepoOrUnscoped},
			{Param: "chatId", Extract: func(e dto.AgentChatEvent) string { return e.ChatID }, Match: ws.ExactMatch},
		},
	}
}

// matchRepoOrUnscoped holds a frame that KNOWS its repo to exactly that repo,
// and lets one that does not reach every subscriber.
//
// The second half is not laxness, it is the honest answer for the rows that
// have no repo to be held to: a folder row carries none, and neither does a
// bubble whose ancestry owns no workspace. Refusing those would drop the live
// folder feed the Chats panel repaints from. ws.ExactMatch would do exactly
// that, which is why this is its own function and not that one.
func matchRepoOrUnscoped(
	param string,
	value string,
) bool {
	return value == "" || param == value
}

// lspDef scopes the LSP diagnostics topic to a single owned session, named
// either way its live routes name one.
//
// ONE StreamDef serves both routes, because there is one Broadcaster: it is
// built once, in New, and every client registers against the same compiled def
// regardless of which route it upgraded on. So the two filters below are not
// alternatives the wiring picks between — both are declared for every client,
// and each client activates whichever one its own request resolves:
//
//   - /projects/:p/repos/:r/workspaces/:wsId/lsp/ws binds :wsId and no
//     :chatId, so the wsId filter is active and scopes it to exactly one
//     workspace's diagnostics, exactly as before this step.
//   - /chats/:chatId/lsp/ws binds :chatId and no :wsId, so the chatId filter
//     is active and scopes it to exactly the diagnostics that chat's own
//     lsp/didOpen etc. calls produced (handlers.Handlers.lspOwnerID) — never a
//     sibling chat's, even one sharing this chat's worktree.
//
// Unlike gitDef/filesDef's chatId filter, this one is NOT a fan-out membership
// match (ExtractSet): editor/LSP is spec §4.2's OWNED bucket, so an event has
// exactly one owner, never a set of chats to reach, and both filters extract
// the very same field.
//
// NEITHER filter is Required, for the same reason gitDef's aren't: Required on
// chatId would refuse every client of the still-live workspace-scoped route,
// which cannot resolve a :chatId; Required on wsId would refuse every
// chat-scoped one.
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
			{Param: "wsId", Extract: func(e lspdomain.DiagnosticsEvent) string { return e.WsID }, Match: ws.ExactMatch},
			{Param: "chatId", Extract: func(e lspdomain.DiagnosticsEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

var _ hub.Subscriber = (*Container)(nil)
