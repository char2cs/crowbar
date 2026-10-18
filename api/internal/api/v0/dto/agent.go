package dto

import (
	"cmp"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatRuntime is the DERIVED process view of one chat: the runner PLACED on it right
// now, and the conversations it has hosted. Neither is chat state — the chat aggregate
// stores no process facts at all — so both are joined on at read time, off the runner
// projections, by the handler that builds the DTO.
//
// LiveRunner is nil exactly when the chat is DORMANT, and nil is a complete answer, not
// a missing one: a live-runner row exists exactly while its PTY does, so its absence IS
// the liveness verdict. That is why nothing here (and nothing on the wire shapes below)
// carries a status/isLive flag — a second authority on liveness could only drift from
// the process, and that drift is the production bug this refactor exists to delete.
type ChatRuntime struct {
	// LiveRunner is the runner currently pointed at the chat, or nil when none is —
	// i.e. when the chat is dormant because its CLI exited or died with the daemon.
	LiveRunner *agents.Runner

	// Conversations is the chat's append-only history, OLDEST FIRST (so the last
	// element is its last conversation). Joined in only for the single-chat detail
	// read; a list leaves it nil.
	Conversations []agents.ChatConversation

	// TerminalWait is the daemon's standing answer to "is this chat's CLI parked
	// on a modal Crowbar cannot answer?". Derived, never stored, and the zero
	// value — not waiting — is both the common case and the answer for every
	// provider that declares no such prompts.
	TerminalWait domain.AgentTerminalWait

	// AttachedSessionID is the terminal session the runner's native view IS
	// right now, set only between a SwitchToTerminal and its matching
	// SwitchToNative. Empty otherwise — never a fact this chat's aggregate
	// stores, same reasoning as TerminalWait.
	AttachedSessionID string

	// Session is the supervisor's record of how the chat's last runner ended.
	Session domain.AgentSession

	// Conversation is the chat's conversation with its current provider, as its
	// ledger holds it; the zero value when it has none.
	Conversation ledger.Conversation

	// Version and Phase come from the chat snapshot owner: the version that
	// orders every answer about this chat, and the lifecycle phase it is in.
	Version int64
	Phase   string
}

// ChatSnapshotRuntime assembles the runtime half of a chat snapshot.
func ChatSnapshotRuntime(
	live *agents.Runner,
	phase string,
	version int64,
	wait domain.AgentTerminalWait,
	attachedSessionID string,
	session domain.AgentSession,
	conversation ledger.Conversation,
) ChatRuntime {
	return ChatRuntime{
		LiveRunner:        live,
		TerminalWait:      wait,
		AttachedSessionID: attachedSessionID,
		Session:           session,
		Conversation:      conversation,
		Version:           version,
		Phase:             phase,
	}
}

// AgentSessionDTO says, while a chat is dormant, how its last runner ended —
// rendered by the client, never inferred.
type AgentSessionDTO struct {
	ExitReason string     `json:"exitReason,omitempty"`
	ExitedAt   *time.Time `json:"exitedAt,omitempty"`
}

// AgentSessionDTOFrom collapses an empty record to nil so the field is absent.
func AgentSessionDTOFrom(s domain.AgentSession) *AgentSessionDTO {
	if s.ExitReason == "" {
		return nil
	}
	out := &AgentSessionDTO{ExitReason: s.ExitReason}
	if !s.ExitedAt.IsZero() {
		at := s.ExitedAt
		out.ExitedAt = &at
	}
	return out
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

// AgentTerminalWaitDTO says a chat's CLI is blocked on a prompt Crowbar has no
// channel to answer, so the only way past it is the terminal.
//
// It is the COMPLEMENT of AgentChoiceDTO. A choice is a prompt that reached
// Crowbar over a hook and can be answered in the chat; this is one that did not
// and cannot, which used to render as nothing at all — no spinner, no message, no
// error, a pane that looked dead over a live process.
//
// Kind is Crowbar's own name for the prompt, and it is OMITTED whenever the daemon
// recognised only that something is up. A client with no kind says exactly that
// ("waiting for input in the terminal") rather than guessing at specifics, and a
// client seeing a kind it does not know does the same — so a daemon that learns a
// new kind never makes an older client say something wrong.
type AgentTerminalWaitDTO struct {
	Kind string `json:"kind,omitempty"`
}

// AgentChatDTO is the wire shape of a Crowbar-owned agentic chat: the workspace it
// belongs to, its title, when it was created — and the three DERIVED runner facts a
// client needs to render and attach to it, joined on from the runner projections
// (they are never stored on the chat).
//
// LiveRunnerID is the whole liveness contract. It names the runner placed on this chat,
// and it is "" exactly when the chat is dormant; a client needs no second call and no
// status field to know whether a pane can attach, because the live row backing it exists
// exactly while the vendor CLI's PTY does. "" here is a MEANINGFUL value (dormant), not
// a placeholder, which is why these fields are always present on the wire rather than
// omitted — the shape is honest either way.
type AgentChatDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`

	// Type is the row's own kind in the sidebar forest (domain.ChatType — chat,
	// branch, folder, workflow), always present rather than omitted: unlike
	// ParentID/Order's ""/0-is-meaningful pattern, "" is not a real ChatType
	// value, but every row this DTO ever serializes has a real Type, so there
	// is no meaningful absent case to distinguish. It is what lets a client
	// tell a locked-branch or repo-home row apart from an ordinary chat within
	// the same GET .../repos/:repoId/chats list.
	Type domain.ChatType `json:"type"`

	// LiveRunnerID is the id of the runner placed on this chat, or "" when the chat
	// is dormant. This IS the liveness answer — do not look for another.
	LiveRunnerID string `json:"liveRunnerId"`

	// TerminalSessionID is that runner's PTY: the terminal session a chat pane
	// attaches to. Empty exactly when LiveRunnerID is — no runner, nothing to attach.
	TerminalSessionID string `json:"terminalSessionId"`

	// Surface is the VIEW this chat is on RIGHT NOW — design spec 2.5,
	// domain.Chat.Surface. Birth seeds it and the two switch calls move it,
	// so it never says how the chat GOT here. OMITTED for the overwhelming
	// majority, which sit on their provider's own default face.
	//
	// The client needs it for one thing: a chat on a hooks-channel surface has
	// NO api connection behind it, so the PTY above IS its conversation and
	// there is nothing to ask for before showing it.
	Surface string `json:"surface,omitempty"`

	// ActiveProviderID is the provider whose CLI is (or last was) talking to this
	// chat: the LIVE runner's provider while the chat is live, else whichever is
	// newer of its most-recently-active conversation or its last durable
	// provider-switch interruption (agents.ActiveProviderID — see there for why
	// both sources are needed). That fallback is what lets a dormant chat still
	// show the right glyph and dropdown selection, and lets Resume know who to
	// bring back. Empty only on a chat no runner has ever been placed on.
	ActiveProviderID string `json:"activeProviderId"`

	// Working is the chat aggregate's folded busy answer. It is included in the
	// snapshot as well as lifecycle frames so reconnect can reseed truth without
	// guessing from the last event kind.
	Working bool `json:"working"`

	// TerminalWait is present exactly while this chat's CLI is blocked on a
	// terminal-only prompt, and OMITTED otherwise — the common case, and the
	// permanent case for every provider declaring no such prompts, so the wire
	// shape for an unaffected chat is byte-identical to what it was before this
	// existed.
	//
	// It rides the snapshot as well as the lifecycle frame for the same reason
	// Working does: a client that connects while a chat is ALREADY parked missed
	// the frame that said so, and would otherwise show a dead pane until the state
	// happened to change.
	TerminalWait *AgentTerminalWaitDTO `json:"terminalWait,omitempty"`

	// ParentID is the row this chat hangs off in the Chats tree — another chat, a
	// folder, or "" at the panel root — and Order is its dense index within that
	// parent's sibling space, which chats SHARE with chat folders.
	//
	// Both are always present rather than omitted, because "" and 0 are both
	// MEANINGFUL: "" is the panel root and 0 is the first slot. A client that had
	// to tell an absent key from a zero value would be guessing at the two
	// commonest placements there are.
	//
	// A parent that names a CHAT means this row is a THREAD of it and reads that
	// chat's turns; one that names a FOLDER is organisation only. The wire shape
	// does not distinguish them: the client already holds both lists and resolves
	// the id against them.
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`

	// Model and Effort are the chat's sticky choice of what to run its provider
	// CLI as, and they are OMITTED when unset because unset is a real answer:
	// "whatever this provider defaults to", which is not any value the client
	// could render as selected. A picker shows no selection for an absent field,
	// which is exactly right — Crowbar does not know what the default resolves to
	// and will not pretend to.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	// LaunchModel and LaunchEffort are what the LIVE runner actually spawned as —
	// AgentRunner.LaunchModel/LaunchEffort, Crowbar's own record of the resolved
	// argv, which can differ from Model/Effort above (a provider or an
	// organization's own config can rewrite a request). '' exactly when
	// LiveRunnerID is: a dormant chat has no live spawn to report.
	LaunchModel  string `json:"launchModel,omitempty"`
	LaunchEffort string `json:"launchEffort,omitempty"`

	// Worktree is the git state of the worktree this chat OWNS — branch, diff
	// counts, lock status, merge and PR state — and is present exactly when
	// WorkspaceID is non-empty (spec §5). It is what lets one read of the chat
	// list answer everything the deleted workspace list used to, in ONE object
	// per row rather than two fetches a client has to join by id.
	//
	// Omitted, not zero, for a chat that owns no worktree: see ChatWorktreeDTO.
	Worktree *ChatWorktreeDTO `json:"worktree,omitempty"`

	CreatedAt time.Time `json:"createdAt"`

	// Version orders every answer the daemon gives about this chat — frame or
	// GET — so a client applies a snapshot only if its version is newer than
	// the one it holds (invariant A6). Larger across daemon restarts too.
	Version int64 `json:"version"`

	// Phase is the chat's lifecycle phase: dormant, starting, live, switching
	// or stopping. The client renders it; it never infers it.
	Phase string `json:"phase"`

	// Session is why the chat is dormant.
	Session *AgentSessionDTO `json:"session,omitempty"`

	// Conversation is the chat's conversation with its current provider,
	// omitted until the chat is bound to a session.
	Conversation *AgentConversationDTO `json:"conversation,omitempty"`
}

// AgentChatDTOFrom converts a persisted AgentChat plus its derived runtime into the
// wire shape. The zero ChatRuntime (no live runner, no history) is the honest shape of
// a chat that has never had a runner: every derived field reads "".
//
// wt is the chat's own worktree state, ALREADY RESOLVED by the caller, and nil
// for a chat that owns none. It is a resolved value rather than a lookup for
// the same reason WorkspaceDTOFrom takes an eligibility instead of computing
// one: resolving it needs the row's repo siblings, and doing that per row would
// put a repo-wide read on the broadcast hot path (spec §10).
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
		Working:          c.Working,
		ParentID:         c.ParentID,
		Order:            c.Order,
		Model:            c.Model,
		Effort:           c.Effort,
		Worktree:         wt,
		CreatedAt:        c.CreatedAt,
		Version:          rt.Version,
		Phase:            rt.Phase,
		Session:          AgentSessionDTOFrom(rt.Session),
		Conversation:     agentConversationDTOFrom(rt.Conversation),
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
// waiting" to nil so the field is absent rather than present-and-false. Absence is
// the honest encoding: a chat is not carrying a prompt it does not have.
func TerminalWaitDTOFrom(
	w domain.AgentTerminalWait,
) *AgentTerminalWaitDTO {
	if !w.Waiting {
		return nil
	}
	return &AgentTerminalWaitDTO{Kind: w.Kind}
}

// PromptSubmissionDTO is the wire shape of domain.AgentPromptSubmission — see
// there for what the pair identifies and why a completed model turn is
// deliberately absent from it.
type PromptSubmissionDTO struct {
	RunnerID          string `json:"runnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
}

// PendingPromptDTO is the wire shape of domain.PendingPrompt — a prompt the
// provider has not acknowledged yet, sent so a client whose own local copy was
// lost can recover the literal text.
type PendingPromptDTO struct {
	Text  string `json:"text"`
	State string `json:"state"`
	// RequestID is the original client request id, carried through so the
	// recovered row rejoins this chat's own dedup and settlement broadcasts
	// instead of a freshly minted id nothing will ever match.
	RequestID string `json:"requestId"`
}

// activeProviderID is the live runner's provider while one is placed (mid-switch
// the incoming runner is already the truth), else the chat's own durable vendor
// — domain.Chat.ProviderID, the single owner of that answer (spec §7-A target 2).
func activeProviderID(
	chat domain.Chat,
	rt ChatRuntime,
) string {
	if rt.LiveRunner != nil {
		return rt.LiveRunner.ProviderID
	}
	return chat.ProviderID
}

// AgentChatDTOList converts a slice of AgentChats into wire DTOs, returning a
// non-nil slice so the envelope carries [] rather than null when there are none.
// runtimes is keyed by chat id; a chat with no entry is rendered from the zero
// ChatRuntime — dormant, no history — which is exactly what a chat missing from both
// runner projections is.
//
// worktreeFn resolves each row's owned worktree, and is the exact counterpart
// of WorkspaceDTOList's eligFn/owningChatIDFn: a closure the CALLER builds over
// the repo-wide reads it has already taken once, so the enrichment costs one
// repo read for the whole list rather than one per row. A nil worktreeFn — the
// honest wiring for a surface that serves only folder rows, which own no
// worktree by construction — leaves every row's Worktree absent.
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

// AgentChatDetailDTO is the wire shape of GET .../repos/:repoId/chats/:id: the
// chat (with its derived runner facts) plus the conversations it has hosted, oldest
// first. Conversations succeeds the deleted `segments` list: it is what a segment really
// was, minus everything that described a process (no status, no PTY, no runner id), so
// it is pure append-only history and cannot drift from reality. It is always non-nil so
// the envelope carries [] rather than null.
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

// HandoffDTO is the wire shape of GET .../repos/:repoId/chats/:id/handoff: the
// assembled handoff blob a freshly spawned provider CLI can be given as prior
// context. Handoff is "" (not omitted) when the chat's ledger has no entries
// yet.
type HandoffDTO struct {
	Handoff string `json:"handoff"`
}
