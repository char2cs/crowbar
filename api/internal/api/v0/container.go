package v0

import (
	"encoding/json"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the v0 delivery surface: the Class-A broadcasters plus REST routes.
// It implements hub.Subscriber so app-layer broadcasts reach connected clients.
type Container struct {
	workspaces *ws.Broadcaster[domain.Workspace]
	chats      *ws.Broadcaster[hub.ChatStatusEvent]
	git        *ws.Broadcaster[gitdomain.GitStatus]
	files      *ws.Broadcaster[domain.FileChangeEvent]
	lsp        *ws.Broadcaster[lspdomain.DiagnosticsEvent]
	app        *app.Container
	eng        *engine.Container
}

// New builds the v0 container and registers it as a hub subscriber.
func New(
	appContainer *app.Container,
	engContainer *engine.Container,
) *Container {
	c := &Container{
		workspaces: ws.NewBroadcaster(workspacesDef()),
		chats:      ws.NewBroadcaster(chatsDef()),
		git:        ws.NewBroadcaster(gitDef()),
		files:      ws.NewBroadcaster(filesDef()),
		lsp:        ws.NewBroadcaster(lspDef()),
		app:        appContainer,
		eng:        engContainer,
	}
	if engContainer != nil && engContainer.LSP != nil {
		engContainer.LSP.OnDiagnostics(c.lsp.Push)
	}
	appContainer.Hub.Register(c)
	return c
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

// PushGit implements hub.Subscriber.
func (c *Container) PushGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	c.git.Push(status)
}

// PushFile implements hub.Subscriber.
func (c *Container) PushFile(
	evt domain.FileChangeEvent,
) {
	c.files.Push(evt)
}

// WaitWorkspacesRegistered blocks until a workspaces client registers. Test-only.
func (c *Container) WaitWorkspacesRegistered() {
	c.workspaces.WaitRegistered()
}

// WaitChatsRegistered blocks until a chats client registers. Test-only.
func (c *Container) WaitChatsRegistered() {
	c.chats.WaitRegistered()
}

// WaitLSPRegistered blocks until an LSP diagnostics client registers. Test-only.
func (c *Container) WaitLSPRegistered() {
	c.lsp.WaitRegistered()
}

// PushLSP fans a diagnostics event out to subscribed clients. Test-only.
func (c *Container) PushLSP(
	evt lspdomain.DiagnosticsEvent,
) {
	c.lsp.Push(evt)
}

func workspacesDef() ws.StreamDef[domain.Workspace] {
	return ws.StreamDef[domain.Workspace]{
		Namespace: func(w domain.Workspace) string { return w.ID },
		Serialize: func(w domain.Workspace) ([]byte, error) { return json.Marshal(w) },
		Filters: []ws.FilterDef[domain.Workspace]{
			{Param: "projectId", Extract: func(w domain.Workspace) string { return w.ProjectID }, Match: ws.ExactMatch},
			{Param: "repoId", Extract: func(w domain.Workspace) string { return w.RepoID }, Match: ws.ExactMatch},
		},
	}
}

func chatsDef() ws.StreamDef[hub.ChatStatusEvent] {
	return ws.StreamDef[hub.ChatStatusEvent]{
		Namespace: func(e hub.ChatStatusEvent) string { return e.ChatID },
		Serialize: func(e hub.ChatStatusEvent) ([]byte, error) { return json.Marshal(e) },
		Filters: []ws.FilterDef[hub.ChatStatusEvent]{
			{Param: "wsId", Extract: func(e hub.ChatStatusEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

func gitDef() ws.StreamDef[gitdomain.GitStatus] {
	return ws.StreamDef[gitdomain.GitStatus]{
		Namespace: func(_ gitdomain.GitStatus) string { return "" },
		Serialize: func(s gitdomain.GitStatus) ([]byte, error) { return json.Marshal(s) },
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

func lspDef() ws.StreamDef[lspdomain.DiagnosticsEvent] {
	return ws.StreamDef[lspdomain.DiagnosticsEvent]{
		Namespace: func(e lspdomain.DiagnosticsEvent) string { return e.WsID },
		Serialize: func(e lspdomain.DiagnosticsEvent) ([]byte, error) { return json.Marshal(e) },
		Filters: []ws.FilterDef[lspdomain.DiagnosticsEvent]{
			{Param: "wsId", Extract: func(e lspdomain.DiagnosticsEvent) string { return e.WsID }, Match: ws.ExactMatch},
		},
	}
}

var _ hub.Subscriber = (*Container)(nil)
