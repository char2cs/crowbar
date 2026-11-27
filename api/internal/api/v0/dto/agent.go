package dto

import (
	"cmp"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ChatRuntime struct {
	LiveRunner *agents.Runner

	Conversations []agents.ChatConversation

	TerminalWait domain.AgentTerminalWait

	AttachedSessionID string

	Conversation ledger.Conversation

	Version int64
	Phase   string

	PendingMove *ledger.Move
	Working     bool
}

func ChatSnapshotRuntime(
	live *agents.Runner,
	phase string,
	version int64,
	wait domain.AgentTerminalWait,
	attachedSessionID string,
	conversation ledger.Conversation,
	working bool,
) ChatRuntime {
	return ChatRuntime{
		Working:           working,
		LiveRunner:        live,
		TerminalWait:      wait,
		AttachedSessionID: attachedSessionID,
		Conversation:      conversation,
		Version:           version,
		Phase:             phase,
	}
}

type PendingMoveDTO struct {
	RequestID   string `json:"requestId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	ParentID    string `json:"parentId,omitempty"`
	Label       string `json:"label,omitempty"`
	BranchOut   bool   `json:"branchOut,omitempty"`
}

func pendingMoveDTOFrom(m *ledger.Move) *PendingMoveDTO {
	if m == nil {
		return nil
	}
	return &PendingMoveDTO{
		RequestID: m.RequestID, WorkspaceID: m.To, ParentID: m.ParentID, Label: m.Label, BranchOut: m.BranchOut,
	}
}

type AgentConversationDTO struct {
	ProviderID string `json:"providerId"`
	SessionID  string `json:"sessionId"`
	State      string `json:"state"`
}

func agentConversationDTOFrom(c ledger.Conversation) *AgentConversationDTO {
	if c.State == "" || c.State == ledger.ConversationUnbound {
		return nil
	}
	return &AgentConversationDTO{ProviderID: c.Provider, SessionID: c.SessionID, State: string(c.State)}
}

type AgentTerminalWaitDTO struct {
	Kind string `json:"kind,omitempty"`

	Answers []AgentTerminalAnswerDTO `json:"answers,omitempty"`
}

type AgentTerminalAnswerDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type AgentChatDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`

	Type domain.ChatType `json:"type"`

	LiveRunnerID string `json:"liveRunnerId"`

	TerminalSessionID string `json:"terminalSessionId"`

	Surface string `json:"surface,omitempty"`

	ActiveProviderID string `json:"activeProviderId"`

	Working bool `json:"working"`

	TerminalWait *AgentTerminalWaitDTO `json:"terminalWait,omitempty"`

	ParentID string `json:"parentId"`
	Order    int    `json:"order"`

	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	PermissionLevel string `json:"permissionLevel,omitempty"`

	LaunchModel  string `json:"launchModel,omitempty"`
	LaunchEffort string `json:"launchEffort,omitempty"`

	Worktree *ChatWorktreeDTO `json:"worktree,omitempty"`

	CreatedAt time.Time `json:"createdAt"`

	Version int64 `json:"version"`

	Phase string `json:"phase"`

	Conversation *AgentConversationDTO `json:"conversation,omitempty"`
	PendingMove  *PendingMoveDTO       `json:"pendingMove,omitempty"`
}

func AgentChatDTOFrom(
	c domain.Chat,
	rt ChatRuntime,
	wt *ChatWorktreeDTO,
) AgentChatDTO {
	out := AgentChatDTO{
		ID:               c.ID,
		WorkspaceID:      c.WorkspaceID,
		Title:            c.Title,
		Type:             c.Type,
		Surface:          c.Surface,
		ActiveProviderID: activeProviderID(c, rt),
		Working:          rt.Working,
		ParentID:         c.ParentID,
		Order:            c.Order,
		Model:            c.Model,
		Effort:           c.Effort,
		PermissionLevel:  pinnedLevel(c),
		Worktree:         wt,
		CreatedAt:        c.CreatedAt,
		Version:          rt.Version,
		Phase:            rt.Phase,
		Conversation:     agentConversationDTOFrom(rt.Conversation),
		PendingMove:      pendingMoveDTOFrom(rt.PendingMove),
	}
	if rt.LiveRunner != nil {
		out.LiveRunnerID = rt.LiveRunner.ID
		out.TerminalSessionID = rt.LiveRunner.TerminalSession
		out.LaunchModel = cmp.Or(rt.LiveRunner.ReportedModel, rt.LiveRunner.LaunchModel)
		out.LaunchEffort = cmp.Or(rt.LiveRunner.ReportedEffort, rt.LiveRunner.LaunchEffort)
		if rt.AttachedSessionID != "" {
			out.TerminalSessionID = rt.AttachedSessionID
		}
	}
	out.TerminalWait = TerminalWaitDTOFrom(rt.TerminalWait)
	return out
}

func TerminalWaitDTOFrom(
	w domain.AgentTerminalWait,
) *AgentTerminalWaitDTO {
	if !w.Waiting {
		return nil
	}
	out := &AgentTerminalWaitDTO{Kind: w.Kind}
	for _, a := range w.Answers {
		out.Answers = append(out.Answers, AgentTerminalAnswerDTO{ID: a.ID, Label: a.Label})
	}
	return out
}

type PromptSubmissionDTO struct {
	RunnerID          string `json:"runnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
}

type AgentRetractDTO struct {
	State string `json:"state"`
	Text  string `json:"text"`
}

func activeProviderID(
	chat domain.Chat,
	rt ChatRuntime,
) string {
	if rt.LiveRunner != nil {
		return rt.LiveRunner.ProviderID
	}
	return chat.ProviderID
}

func AgentChatDTOList(
	chats []domain.Chat,
	runtimes map[string]ChatRuntime,
	worktreeFn func(domain.Chat) *ChatWorktreeDTO,
) []AgentChatDTO {
	out := make([]AgentChatDTO, 0, len(chats))
	for _, c := range chats {
		var wt *ChatWorktreeDTO
		if worktreeFn != nil {
			wt = worktreeFn(c)
		}
		out = append(out, AgentChatDTOFrom(c, runtimes[c.ID], wt))
	}
	return out
}

type AgentChatDetailDTO struct {
	AgentChatDTO
	Conversations []agents.ChatConversation `json:"conversations"`
}

func AgentChatDetailDTOFrom(
	c domain.Chat,
	rt ChatRuntime,
	wt *ChatWorktreeDTO,
) AgentChatDetailDTO {
	convs := rt.Conversations
	if convs == nil {
		convs = []agents.ChatConversation{}
	}
	return AgentChatDetailDTO{
		AgentChatDTO:  AgentChatDTOFrom(c, rt, wt),
		Conversations: convs,
	}
}

type HandoffDTO struct {
	Handoff string `json:"handoff"`
}

func pinnedLevel(c domain.Chat) string {
	if !c.PermissionLevelExplicit {
		return ""
	}
	return c.PermissionLevel
}
