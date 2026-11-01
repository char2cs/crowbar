package v0

import (
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// PushAgentChatEvent implements hub.Subscriber: one chat snapshot frame,
// scoped FRESH — a snapshot follows every structural change (a create, a
// placement, a runner arriving), each of which can move a bubble into another
// repo.
func (c *Container) PushAgentChatEvent(ev dto.AgentChatEvent) {
	scope := c.freshAgentChatScope(ev.ChatID, ev.WorkspaceID)
	ev.ProjectID, ev.RepoID = scope.ProjectID, scope.RepoID
	c.agentChats.Push(ev)
}

// PushAgentChatTelemetry implements hub.Subscriber, on the SAME
// workspace-scoped agent-chat WebSocket, scoped like every other chat frame.
func (c *Container) PushAgentChatTelemetry(
	chatID string,
	workspaceID string,
	report agents.Telemetry,
) {
	out := dto.AgentTelemetryDTOFrom(report)
	scope := c.agentChatScope(chatID, workspaceID)
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		ProjectID:   scope.ProjectID,
		RepoID:      scope.RepoID,
		Kind:        dto.AgentChatKindTelemetry,
		Telemetry:   &out,
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
// A folder announced with a workspace resolves that workspace's project and
// repo like any other row, so a project-home folder is now held to its own
// project rather than fanned out across every one of them. A folder announced
// with NO workspace — the repo mount's own folder routes bind no :wsId — still
// resolves nothing and reaches every subscriber (matchScopeOrUnscoped): the
// folder half of the repo boundary is the disclosed limitation
// ChatTreeUsecase.ListInRepo already carries, unchanged here, and the frame
// carries an id and a kind rather than any of the conversation's content. The
// fresh resolution is taken anyway for its other half: a folder move is a
// structural change that can have carried bubbles into another repo with it,
// and the memo those bubbles' streaming frames read must not survive it.
func (c *Container) PushAgentChatFolder(
	folderID string,
	workspaceID string,
	kind string,
) {
	scope := c.freshAgentChatScope(folderID, workspaceID)
	c.agentChats.Push(dto.AgentChatEvent{
		FolderID:    folderID,
		WorkspaceID: workspaceID,
		ProjectID:   scope.ProjectID,
		RepoID:      scope.RepoID,
		Kind:        kind,
	})
}
