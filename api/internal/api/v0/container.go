package v0

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/core/logring"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container is the v0 delivery surface: the topics the stream does not carry
// yet plus REST routes. It implements hub.Subscriber so app-layer broadcasts
// reach connected clients.
type Container struct {
	projects   *ws.Broadcaster[dto.ProjectDTO]
	repos      *ws.Broadcaster[dto.RepoDTO]
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
		chatScopes: newAgentChatScopes(),
		app:        appContainer,
		eng:        engContainer,
	}
	for _, option := range options {
		option(c)
	}
	c.agentChats = ws.NewBroadcaster(agentChatDef())
	if engContainer != nil && engContainer.Terminal != nil {
		if appContainer.Usecases != nil && appContainer.Usecases.TerminalMeta != nil {
			engContainer.Terminal.SetMetaStore(appContainer.Usecases.TerminalMeta)
		}
	}
	appContainer.Hub.Register(c)
	return c
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
