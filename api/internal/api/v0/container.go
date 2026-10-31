package v0

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the v0 delivery surface: the seven realtime topics plus REST
// routes. It implements hub.Subscriber so app-layer broadcasts reach connected
// clients.
//
// The push-only Broadcaster[T] instances held here are git, files, and lsp. The
// Terminal topic is intentionally NOT a Broadcaster[T]: PTY streams are
// bidirectional, so the Terminal topic is served by the engine.Attach WebSocket
// handler (endpoints/terminal/handlers/ws.go), whose ring-buffer replay is its
// snapshot-on-subscribe (03 §1a). It is wired separately in router.go.
type Container struct {
	projects   *ws.Broadcaster[dto.ProjectDTO]
	repos      *ws.Broadcaster[dto.RepoDTO]
	threads    *ws.Broadcaster[dto.ThreadDTO]
	terminals  *ws.Broadcaster[dto.TerminalSessionDTO]
	git        *ws.Broadcaster[gitdomain.GitStatusEvent]
	files      *ws.Broadcaster[domain.FileChangeEvent]
	documents  *ws.Broadcaster[domain.FileChangeEvent]
	lsp        *ws.Broadcaster[lspdomain.DiagnosticsEvent]
	agentChats *ws.Broadcaster[dto.AgentChatEvent]
	// chatScopes answers what each agent-chat frame namespaces under — see
	// agent_chat_scope.go.
	chatScopes *agentChatScopes
	app        *app.Container
	eng        *engine.Container
	// logs backs GET /console/logs; nil leaves the route unmounted.
	logs logring.Ring
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
	options ...Option,
) *Container {
	if appContainer == nil {
		panic("v0: appContainer is required")
	}
	c := &Container{
		projects:   ws.NewBroadcaster(projectsDef(appContainer)),
		repos:      ws.NewBroadcaster(reposDef(appContainer)),
		threads:    ws.NewBroadcaster(threadsDef(appContainer)),
		terminals:  ws.NewBroadcaster(terminalsDef(appContainer, engContainer)),
		git:        ws.NewBroadcaster(withOriginSyncLifecycle(withWatcherLifecycle(gitDef(appContainer), appContainer), appContainer)),
		files:      ws.NewBroadcaster(withWatcherLifecycle(filesDef(appContainer), appContainer)),
		documents:  ws.NewBroadcaster(withHostWatchLifecycle(documentsDef(), appContainer)),
		lsp:        ws.NewBroadcaster(withLSPLifecycle(lspDef(appContainer, engContainer), appContainer)),
		chatScopes: newAgentChatScopes(),
		app:        appContainer,
		eng:        engContainer,
	}
	for _, option := range options {
		option(c)
	}
	agentChatStream := withChatProviderPollLifecycle(agentChatDef(), appContainer)
	agentChatStream.Snapshot = c.agentChatSnapshot
	c.agentChats = ws.NewBroadcaster(agentChatStream)
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

// PushWorkspace implements hub.Subscriber. It fans a workspace's state onto the
// CHAT topic alone: the workspace-keyed topic it also served is gone with the
// routes that mounted it (spec §7.4), so the chat that owns the worktree is the
// only scoping answer left.
func (c *Container) PushWorkspace(
	w dto.WorkspaceDTO,
) {
	c.pushChatWorktree(w)
}

// pushChatWorktree fans a workspace's git state out on the CHAT feed, keyed on
// the chat that owns it.
//
// It reads the owning chat id off the frame rather than resolving one, and that
// is deliberate: enrichFrame has already resolved it, through the same
// branch-preferring domain.ResolveOwningChat every other surface uses, so taking
// it here means the chat frame names exactly the row the workspace frame says
// owns this worktree — never a second, independently derived answer that could
// pick a different one.
//
// A workspace with no resolved owning chat pushes nothing. That is the honest
// answer rather than a broadcast to nobody: such a row is the orphan spec §0
// diagnosed, with no chat for a client to draw it on — chat-first creation
// (MintOwningChat/AttachOwningWorkspace, owning_chat.go) is what makes this
// unrepresentable for every workspace made going forward.
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
		ProjectID:   w.ProjectID,
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

// PushGit implements hub.Subscriber. The event names only its workspace: each
// chat-scoped client matches it against its own bound workspace, so a push
// reads no chat or placement state.
func (c *Container) PushGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	// A clean tree carries a nil Files slice; normalise so the frame
	// serialises with files: [] (never null), matching the REST DTO.
	if status.Files == nil {
		status.Files = []gitdomain.GitFile{}
	}
	c.git.Push(gitdomain.GitStatusEvent{WsID: wsID, Status: status})
}

// PushFile implements hub.Subscriber; like PushGit it reads no chat state. A
// host document's change goes to the clients holding it, never to a worktree's.
func (c *Container) PushFile(
	evt domain.FileChangeEvent,
) {
	if evt.Authority == domain.DocumentAuthorityHost {
		c.documents.Push(evt)
		return
	}
	c.files.Push(evt)
}

// PushPlacementChanged implements hub.Subscriber: a chat may now resolve to
// another worktree, so both worktree streams re-resolve their bound clients.
func (c *Container) PushPlacementChanged() {
	c.git.RequestRebind()
	c.files.RequestRebind()
}

// Close stops the worktree streams' rebind workers. Call it once no placement
// broadcast can arrive, before the stores their resolver reads are closed.
func (c *Container) Close() {
	c.git.Close()
	c.files.Close()
}

var _ hub.Subscriber = (*Container)(nil)
