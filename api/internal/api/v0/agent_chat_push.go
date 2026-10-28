package v0

import (
	"context"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// PushAgentChatPromptSettled implements hub.Subscriber, on the SAME
// workspace-scoped agent-chat WebSocket as every other fact about a conversation.
func (c *Container) PushAgentChatPromptSettled(
	chatID string,
	workspaceID string,
	requestID string,
	consumed bool,
) {
	scope := c.agentChatScope(chatID, workspaceID)
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:          chatID,
		WorkspaceID:     workspaceID,
		ProjectID:       scope.ProjectID,
		RepoID:          scope.RepoID,
		Kind:            dto.AgentChatKindPromptSettled,
		ClientRequestID: requestID,
		PromptConsumed:  consumed,
	})
}

// PushAgentChatMessageDelta implements hub.Subscriber, on the SAME
// workspace-scoped agent-chat WebSocket as every other conversation fact.
func (c *Container) PushAgentChatMessageDelta(
	chatID string,
	workspaceID string,
	messageID string,
	text string,
	kind string,
	startedAt time.Time,
) {
	scope := c.agentChatScope(chatID, workspaceID)
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		ProjectID:   scope.ProjectID,
		RepoID:      scope.RepoID,
		Kind:        dto.AgentChatKindMessageDelta,
		Message:     &dto.AgentStreamingMessageDTO{ID: messageID, Text: text, Kind: kind, StartedAt: startedAt},
	})
}

// PushAgentChatPlan implements hub.Subscriber, on the SAME workspace-scoped
// agent-chat WebSocket as every other conversation fact — and scoped like every
// other one.
//
// It set no scope at all for its first several months, so every plan frame went
// out repo-less no matter which chat wrote it and the filter's "a frame with no
// scope to be held to reaches everyone" hatch fired for all of them: the agent's
// own free-text to-do list reached every repo-scoped subscriber on the daemon.
func (c *Container) PushAgentChatPlan(
	chatID string,
	workspaceID string,
	steps []agents.PlanStep,
) {
	out := make([]dto.AgentPlanStepDTO, 0, len(steps))
	for _, s := range steps {
		out = append(out, dto.AgentPlanStepDTO{Text: s.Text, Status: s.Status})
	}
	scope := c.agentChatScope(chatID, workspaceID)
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		ProjectID:   scope.ProjectID,
		RepoID:      scope.RepoID,
		Kind:        dto.AgentChatKindPlan,
		Plan:        out,
	})
}

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

// PushAgentChatCompaction implements hub.Subscriber, on the SAME
// workspace-scoped agent-chat WebSocket as every other conversation fact.
// active picks which of the two kinds rides — see dto.AgentChatKindCompactionStarted's
// own doc comment for why two kinds and no extra field.
//
// It carried no scope either, for the same reason PushAgentChatPlan did not:
// see that method's own comment.
func (c *Container) PushAgentChatCompaction(
	chatID string,
	workspaceID string,
	active bool,
) {
	kind := dto.AgentChatKindCompactionStopped
	if active {
		kind = dto.AgentChatKindCompactionStarted
	}
	scope := c.agentChatScope(chatID, workspaceID)
	c.agentChats.Push(dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		ProjectID:   scope.ProjectID,
		RepoID:      scope.RepoID,
		Kind:        kind,
	})
}

// PushAgentChatChoices implements hub.Subscriber, on the SAME workspace-scoped
// agent-chat WebSocket as every other conversation fact, scoped like each one.
func (c *Container) PushAgentChatChoices(
	chatID string,
	workspaceID string,
	choices []dto.AgentChoiceDTO,
) {
	c.agentChats.Push(c.choiceEvent(chatID, workspaceID, choices))
}

// agentChatSnapshot is what a socket is told the moment it connects: the prompts
// every chat is already blocked on, which no frame will announce again. Each
// frame is held to the new client's own scope by the stream's filters.
func (c *Container) agentChatSnapshot(string) []dto.AgentChatEvent {
	if c.app.Usecases == nil || c.app.Usecases.AgentTurn == nil {
		return nil
	}
	sets, err := c.app.Usecases.AgentTurn.PendingChoiceSets(context.Background())
	if err != nil {
		slog.Warn("agent chats: snapshot pending choices", "err", err)
		return nil
	}
	out := make([]dto.AgentChatEvent, 0, len(sets))
	for _, set := range sets {
		out = append(out, c.choiceEvent(
			set.ChatID, set.WorkspaceID, dto.AgentChoiceDTOsFrom(set.Choices, set.Answerable),
		))
	}
	return out
}

// choiceEvent is one chat's prompts as a frame; it is also what a new socket's
// snapshot replays (see agentChatSnapshot).
func (c *Container) choiceEvent(
	chatID string,
	workspaceID string,
	choices []dto.AgentChoiceDTO,
) dto.AgentChatEvent {
	scope := c.agentChatScope(chatID, workspaceID)
	return dto.AgentChatEvent{
		ChatID:      chatID,
		WorkspaceID: workspaceID,
		ProjectID:   scope.ProjectID,
		RepoID:      scope.RepoID,
		Kind:        dto.AgentChatKindChoice,
		Choices:     choices,
	}
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
