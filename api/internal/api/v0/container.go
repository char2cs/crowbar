package v0

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

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
// Six of the seven topics (03 §3) are push-only Broadcaster[T] instances held
// here: workspaces, chats, git, files, lsp, and chatStream. The seventh topic,
// Terminal, is intentionally NOT a Broadcaster[T]: PTY streams are
// bidirectional, so the Terminal topic is served by the engine.Attach WebSocket
// handler (endpoints/terminal/handlers/ws.go), whose ring-buffer replay is its
// snapshot-on-subscribe (03 §1a). It is wired separately in router.go.
type Container struct {
	workspaces *ws.Broadcaster[domain.Workspace]
	chats      *ws.Broadcaster[hub.ChatStatusEvent]
	git        *ws.Broadcaster[gitdomain.GitStatusEvent]
	files      *ws.Broadcaster[domain.FileChangeEvent]
	lsp        *ws.Broadcaster[lspdomain.DiagnosticsEvent]
	chatStream *ws.Broadcaster[ChatFrame]
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
		workspaces: ws.NewBroadcaster(workspacesDef(appContainer)),
		chats:      ws.NewBroadcaster(chatsDef(appContainer)),
		git:        ws.NewBroadcaster(withWatcherLifecycle(gitDef(appContainer), appContainer)),
		files:      ws.NewBroadcaster(withWatcherLifecycle(filesDef(), appContainer)),
		lsp:        ws.NewBroadcaster(withLSPLifecycle(lspDef(appContainer, engContainer), appContainer)),
		chatStream: ws.NewBroadcaster(chatStreamDef()),
		app:        appContainer,
		eng:        engContainer,
	}
	if engContainer != nil && engContainer.LSP != nil {
		engContainer.LSP.OnDiagnostics(c.lsp.Push)
	}
	appContainer.Hub.Register(c)
	return c
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
// StreamDef, scoping the refcount by wsId resolved from the path or query and
// delegating to the app-layer realtime service.
func withLSPLifecycle[T any](
	def ws.StreamDef[T],
	appContainer *app.Container,
) ws.StreamDef[T] {
	def.ScopeKey = scopeWsID
	def.OnSubscribe = appContainer.Realtime.AcquireLSP
	def.OnUnsubscribe = appContainer.Realtime.ReleaseLSP
	return def
}

// scopeWsID resolves the workspace id from the path param, falling back to the
// query param, mirroring the dual-served Git/Files/LSP routes (T15).
func scopeWsID(
	c *gin.Context,
) string {
	if id := c.Param("wsId"); id != "" {
		return id
	}
	return c.Query("wsId")
}

// PushWorkspace implements hub.Subscriber.
func (c *Container) PushWorkspace(
	wsRow domain.Workspace,
) {
	c.workspaces.Push(wsRow)
}

// PushChat implements hub.Subscriber.
func (c *Container) PushChat(
	evt hub.ChatStatusEvent,
) {
	c.chats.Push(evt)
}

// PushGit implements hub.Subscriber. It wraps the status in a wsId-carrying
// event so the Git broadcaster can scope the fan-out to a single workspace.
func (c *Container) PushGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	c.git.Push(gitdomain.GitStatusEvent{WsID: wsID, Status: status})
}

// PushFile implements hub.Subscriber.
func (c *Container) PushFile(
	evt domain.FileChangeEvent,
) {
	c.files.Push(evt)
}

func workspacesDef(
	appContainer *app.Container,
) ws.StreamDef[domain.Workspace] {
	return ws.StreamDef[domain.Workspace]{
		Namespace: func(w domain.Workspace) string { return w.ID },
		Serialize: func(w domain.Workspace) ([]byte, error) { return json.Marshal(w) },
		Snapshot:  workspacesSnapshot(appContainer),
		Filters: []ws.FilterDef[domain.Workspace]{
			{Param: "projectId", Extract: func(w domain.Workspace) string { return w.ProjectID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(w domain.Workspace) string { return w.RepoID }, Match: ws.ExactMatch},
		},
	}
}

func chatsDef(
	appContainer *app.Container,
) ws.StreamDef[hub.ChatStatusEvent] {
	return ws.StreamDef[hub.ChatStatusEvent]{
		Namespace: func(e hub.ChatStatusEvent) string { return e.ChatID },
		Serialize: func(e hub.ChatStatusEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:  chatsSnapshot(appContainer),
		Filters: []ws.FilterDef[hub.ChatStatusEvent]{
			{Param: "wsId", Extract: func(e hub.ChatStatusEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

// gitDef scopes the Git topic to a single workspace by wsId. The wsId resolves
// from the PATH param on the dual-served /v0/workspaces/:wsId/git/status route
// and from the QUERY param on the dedicated /v0/ws/git?wsId= route. The wire
// payload is a bare GitStatus (the embedded Status), matching the REST snapshot
// of the dual-serve route; only the WsID is used for filtering, never serialized
// onto the Git stream.
func gitDef(
	appContainer *app.Container,
) ws.StreamDef[gitdomain.GitStatusEvent] {
	return ws.StreamDef[gitdomain.GitStatusEvent]{
		Namespace: func(e gitdomain.GitStatusEvent) string { return e.WsID },
		Serialize: func(e gitdomain.GitStatusEvent) ([]byte, error) { return json.Marshal(e.Status) },
		Snapshot:  gitSnapshot(appContainer),
		Filters: []ws.FilterDef[gitdomain.GitStatusEvent]{
			{Param: "wsId", Extract: func(e gitdomain.GitStatusEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

func filesDef() ws.StreamDef[domain.FileChangeEvent] {
	return ws.StreamDef[domain.FileChangeEvent]{
		Namespace: func(e domain.FileChangeEvent) string { return e.WsID },
		Serialize: func(e domain.FileChangeEvent) ([]byte, error) { return json.Marshal(e) },
		Filters: []ws.FilterDef[domain.FileChangeEvent]{
			{Param: "wsId", Extract: func(e domain.FileChangeEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

func lspDef(
	appContainer *app.Container,
	engContainer *engine.Container,
) ws.StreamDef[lspdomain.DiagnosticsEvent] {
	return ws.StreamDef[lspdomain.DiagnosticsEvent]{
		Namespace: func(e lspdomain.DiagnosticsEvent) string { return e.WsID },
		Serialize: func(e lspdomain.DiagnosticsEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:  lspSnapshot(appContainer, engContainer),
		Filters: []ws.FilterDef[lspdomain.DiagnosticsEvent]{
			{Param: "wsId", Extract: func(e lspdomain.DiagnosticsEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

// chatStreamDef scopes the post-spike ChatStream topic per chat by chatId
// (03 §4.3). The topic and its route exist so the topology is complete, but no
// producer ever pushes a ChatFrame to it yet (03 §8) — it is a placeholder.
func chatStreamDef() ws.StreamDef[ChatFrame] {
	return ws.StreamDef[ChatFrame]{
		Namespace: func(f ChatFrame) string { return f.ChatID },
		Serialize: func(f ChatFrame) ([]byte, error) { return json.Marshal(f) },
		Filters: []ws.FilterDef[ChatFrame]{
			{Param: "chatId", Extract: func(f ChatFrame) string { return f.ChatID }, Match: ws.ExactMatch},
		},
	}
}

var _ hub.Subscriber = (*Container)(nil)
