package v0

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container is the v0 delivery surface: the Class-A broadcasters plus REST routes.
// It implements hub.Subscriber so app-layer broadcasts reach connected clients.
type Container struct {
	workspaces *ws.Broadcaster[domain.Workspace]
	chats      *ws.Broadcaster[hub.ChatStatusEvent]
}

// New builds the v0 container and registers it as a hub subscriber.
func New(
	appContainer *app.Container,
) *Container {
	c := &Container{
		workspaces: ws.NewBroadcaster(workspacesDef()),
		chats:      ws.NewBroadcaster(chatsDef()),
	}
	appContainer.Hub.Register(c)
	return c
}

// Register mounts the v0 REST and WebSocket routes.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	registerHealth(rg)
	rg.GET("/ws/workspaces", c.workspaces.Handle)
	rg.GET("/ws/chats", c.chats.Handle)
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

// WaitWorkspacesRegistered blocks until a workspaces client registers. Test-only.
func (c *Container) WaitWorkspacesRegistered() {
	c.workspaces.WaitRegistered()
}

// WaitChatsRegistered blocks until a chats client registers. Test-only.
func (c *Container) WaitChatsRegistered() {
	c.chats.WaitRegistered()
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

var _ hub.Subscriber = (*Container)(nil)
