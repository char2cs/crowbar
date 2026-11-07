package dto

import (
	"cmp"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatRuntime is the derived process view of one chat, joined on at read time because
// the chat aggregate stores no process facts. A nil LiveRunner means dormant: the live
// row exists exactly while its PTY does, so no status flag exists to drift from it.
type ChatRuntime struct {
	// LiveRunner is the runner currently pointed at the chat, or nil when none is —
	// i.e. when the chat is dormant because its CLI exited or died with the daemon.
	LiveRunner *agents.Runner

	// Conversations is the chat's append-only history, OLDEST FIRST (so the last
	// element is its last conversation). Joined in only for the single-chat detail
	// read; a list leaves it nil.
	Conversations []agents.ChatConversation

	// TerminalWait says whether the chat's CLI is parked on a modal Crowbar cannot
	// answer. Derived, never stored; the zero value means not waiting.
	TerminalWait domain.AgentTerminalWait

	// AttachedSessionID is the terminal session the runner's native view is, set
	// only between a SwitchToTerminal and its matching SwitchToNative.
	AttachedSessionID string

	// Conversation is the chat's conversation with its current provider, as its
	// ledger holds it; the zero value when it has none.
	Conversation ledger.Conversation

	// Version and Phase come from the chat snapshot owner: the version that
	// orders every answer about this chat, and the lifecycle phase it is in.
	Version int64
	Phase   string

	// PendingMove is the move waiting for the chat's turn to end; nil when none.
	PendingMove *ledger.Move
	// Working is the chat's ledger saying it is busy.
	Working bool
}

// ChatSnapshotRuntime assembles the runtime half of a chat snapshot.
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

// PendingMoveDTO is a move waiting for the chat's turn boundary.
type PendingMoveDTO struct {
	RequestID   string `json:"requestId"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	ParentID    string `json:"parentId,omitempty"`
	Label       string `json:"label,omitempty"`
	BranchOut   bool   `json:"branchOut,omitempty"`
	Branch      string `json:"branch,omitempty"`
	// AwaitsConsent: the agent asked for it, and it waits for the person to allow or deny it.
	AwaitsConsent bool `json:"awaitsConsent,omitempty"`
}

func pendingMoveDTOFrom(m *ledger.Move) *PendingMoveDTO {
	if m == nil {
		return nil
	}
	return &PendingMoveDTO{
		RequestID: m.RequestID, WorkspaceID: m.To, ParentID: m.ParentID, Label: m.Label, BranchOut: m.BranchOut,
		Branch: m.Branch, AwaitsConsent: m.AwaitsConsent,
	}
}

// AgentConversationDTO is the chat's conversation with one provider: the
// session it resumes by, and whether that session is bound or lost.
type AgentConversationDTO struct {
	ProviderID string `json:"providerId"`
	SessionID  string `json:"sessionId"`
	State      string `json:"state"`
}

// agentConversationDTOFrom is nil for a chat bound to no session yet.
func agentConversationDTOFrom(c ledger.Conversation) *AgentConversationDTO {
	if c.State == "" || c.State == ledger.ConversationUnbound {
		return nil
	}
	return &AgentConversationDTO{ProviderID: c.Provider, SessionID: c.SessionID, State: string(c.State)}
}

// AgentTerminalWaitDTO says a chat's CLI is blocked on a prompt only the terminal can
// answer (the complement of AgentChoiceDTO). Kind is omitted when the prompt is not
// recognised; clients treat a missing or unknown kind as a generic terminal wait.
type AgentTerminalWaitDTO struct {
	Kind string `json:"kind,omitempty"`

	// Answers are one-tap replies the provider's descriptor declares for this
	// prompt; empty means the terminal is the only way past it.
	Answers []AgentTerminalAnswerDTO `json:"answers,omitempty"`
}

// AgentTerminalAnswerDTO is one such reply; the keys that type it stay in the daemon.
type AgentTerminalAnswerDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// AgentChatDTO is the wire shape of a Crowbar-owned agentic chat plus its derived runner
// facts. LiveRunnerID is the whole liveness contract: "" means dormant, so the runner
// fields are always present rather than omitted.
type AgentChatDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`

	// Type is the row's kind in the sidebar forest (chat, branch, folder, workflow);
	// it lets a client tell branch and repo-home rows from ordinary chats in one list.
	Type domain.ChatType `json:"type"`

	// LiveRunnerID is the id of the runner placed on this chat, or "" when the chat
	// is dormant. This IS the liveness answer — do not look for another.
	LiveRunnerID string `json:"liveRunnerId"`

	// TerminalSessionID is that runner's PTY: the terminal session a chat pane
	// attaches to. Empty exactly when LiveRunnerID is — no runner, nothing to attach.
	TerminalSessionID string `json:"terminalSessionId"`

	// Surface is the view this chat is on now, omitted on the provider's default face.
	// A chat on a hooks-channel surface has no api connection, so its PTY is its conversation.
	Surface string `json:"surface,omitempty"`

	// ActiveProviderID is the live runner's provider, else the chat's durable provider,
	// so a dormant chat still shows the right glyph and Resume knows whom to bring back.
	ActiveProviderID string `json:"activeProviderId"`

	// Working is the chat's ledger saying it is busy. It rides the snapshot so
	// reconnect can reseed truth without guessing from the last event kind.
	Working bool `json:"working"`

	// TerminalWait is present only while the CLI is blocked on a terminal-only prompt.
	// It rides the snapshot too so a client connecting mid-wait doesn't show a dead pane.
	TerminalWait *AgentTerminalWaitDTO `json:"terminalWait,omitempty"`

	// ParentID is the chat or folder this row hangs off ("" is the panel root) and Order its
	// dense index among siblings shared with folders; never omitted since ""/0 are meaningful.
	// A chat parent makes this row a thread of it; the client resolves which kind the id is.
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`

	// Model and Effort are the chat's sticky launch choice, omitted when unset: unset
	// means the provider default, which Crowbar cannot name, so a picker shows none.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	// PermissionLevel is the level the chat is pinned to; omitted while it inherits
	// the default, which GET /v0/settings/chat/permission-level reports.
	PermissionLevel string `json:"permissionLevel,omitempty"`

	// LaunchModel and LaunchEffort are what the live runner reports or was spawned as,
	// which provider or org config can make differ from Model/Effort; empty when dormant.
	LaunchModel  string `json:"launchModel,omitempty"`
	LaunchEffort string `json:"launchEffort,omitempty"`

	// Worktree is the git state of the worktree this chat owns, present exactly when
	// WorkspaceID is non-empty, so one chat-list read needs no second fetch to join.
	Worktree *ChatWorktreeDTO `json:"worktree,omitempty"`

	CreatedAt time.Time `json:"createdAt"`

	// Version orders every answer about this chat, frame or GET, so a client applies
	// only newer snapshots. It keeps increasing across daemon restarts.
	Version int64 `json:"version"`

	// Phase is the chat's lifecycle phase: dormant, starting, live, switching
	// or stopping. The client renders it; it never infers it.
	Phase string `json:"phase"`

	// Conversation is the chat's conversation with its current provider,
	// omitted until the chat is bound to a session.
	Conversation *AgentConversationDTO `json:"conversation,omitempty"`
	// PendingMove is where the chat moves once its turn ends.
	PendingMove *PendingMoveDTO `json:"pendingMove,omitempty"`
}

// AgentChatDTOFrom converts a chat plus its derived runtime into the wire shape. wt is
// resolved by the caller (nil when the chat owns no worktree) because resolving it needs
// repo siblings, and a per-row repo read would sit on the broadcast hot path.
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
		// The native view the user switched to is the runner's process now.
		if rt.AttachedSessionID != "" {
			out.TerminalSessionID = rt.AttachedSessionID
		}
	}
	out.TerminalWait = TerminalWaitDTOFrom(rt.TerminalWait)
	return out
}

// TerminalWaitDTOFrom maps the derived verdict onto the wire, collapsing "not
// waiting" to nil so the field is absent rather than present-and-false.
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

// PromptSubmissionDTO is the wire shape of domain.AgentPromptSubmission — see
// there for what the pair identifies and why a completed model turn is
// deliberately absent from it.
type PromptSubmissionDTO struct {
	RunnerID          string `json:"runnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
}

// AgentRetractDTO is a retract's answer: "retracted" (the provider never took
// the prompt) or "interrupted" (its turn was ended), and the prompt's text.
type AgentRetractDTO struct {
	State string `json:"state"`
	Text  string `json:"text"`
}

// activeProviderID is the live runner's provider while one is placed (mid-switch the
// incoming runner is already the truth), else the chat's durable domain.Chat.ProviderID.
func activeProviderID(
	chat domain.Chat,
	rt ChatRuntime,
) string {
	if rt.LiveRunner != nil {
		return rt.LiveRunner.ProviderID
	}
	return chat.ProviderID
}

// AgentChatDTOList converts chats into a non-nil DTO slice; a chat missing from runtimes
// renders dormant. worktreeFn closes over repo reads the caller took once (one read per
// list, not per row); nil leaves every Worktree absent, as for folder-only surfaces.
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

// AgentChatDetailDTO is the wire shape of GET .../repos/:repoId/chats/:id: the chat
// plus the conversations it has hosted, oldest first. Conversations is append-only
// history with no process facts, and always non-nil.
type AgentChatDetailDTO struct {
	AgentChatDTO
	Conversations []agents.ChatConversation `json:"conversations"`
}

// AgentChatDetailDTOFrom composes a chat and its derived runtime into the detail wire
// shape, normalising nil conversations to [] so the envelope never carries null.
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

// HandoffDTO is the wire shape of GET .../repos/:repoId/chats/:id/handoff: the prior
// context a freshly spawned provider CLI can be given; "" when the ledger is empty.
type HandoffDTO struct {
	Handoff string `json:"handoff"`
}

// pinnedLevel is the level c is pinned to, "" while it only inherits the default.
func pinnedLevel(c domain.Chat) string {
	if !c.PermissionLevelExplicit {
		return ""
	}
	return c.PermissionLevel
}
