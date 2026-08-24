package domain

import "time"

// Chat is the Crowbar-owned agentic conversation thread. Mutated only
// through asynx commands. Conversation content lives in the ledger, not here —
// this aggregate holds identity, title, live turn state, and a ledger cursor.
//
// It knows NOTHING about processes. `Segments []AgentSegment` and
// `ActiveSegmentID` are gone, and nothing replaces them on the aggregate (spec
// §2): a chat does not own the CLI that happens to be talking to it. The runner
// points at the chat, never the reverse, so "is this chat live?" is a QUERY
// against the runner read model (runner.LiveRunnerForChat — a row exists
// exactly while its PTY does), never a flag stored here that could contradict
// the process. Which conversations a chat has hosted is likewise a PROJECTION of
// runner events (agentrunner's append-only chat_conversations), not chat state —
// so a conversation switch writes ONE aggregate (the runner) and the chat being
// left is never written to at all. That is what makes the torn cross-aggregate
// write that bricked a chat unrepresentable.
type Chat struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Title       string    `json:"title"`
	TitleLocked bool      `json:"titleLocked"`
	CreatedAt   time.Time `json:"createdAt"`

	// Model and Effort are the chat's STICKY choice of what to run its provider
	// CLI as: durable config beside the title, not a property of any process. They
	// persist between messages so the picker has a value to show and the next
	// message runs under the same choice as the last.
	//
	// EMPTY means "the provider's own default", and that is a distinct fact from
	// any declared value — never a stand-in for one. Nothing substitutes a default
	// into a spawn, so a chat that has chosen nothing produces byte-identical argv
	// to one created before this field existed.
	//
	// They are what the chat WANTS. What its live CLI is actually running is
	// AgentRunner.LaunchModel/LaunchEffort — Crowbar's record of the spawn — and
	// the gap between the two is exactly what makes the next prompt restart the
	// TUI.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	// ParentID is the row this chat hangs off in the Chats tree: another chat, a
	// folder, or "" at the panel root. It is the ONLY record of the relationship
	// — no badge, no stored fork point — because the relationship is live rather
	// than a snapshot taken at a moment.
	//
	// A chat parent means a THREAD: this chat reads that chat's turns, and its
	// chat ancestors' in turn, as they stand whenever it asks. A folder parent
	// means organisation only; folders hold no turns, so lineage steps straight
	// through them. That single field therefore answers both "where is this row
	// drawn" and "what does this agent read", and it is why a drag in this panel
	// legitimately rewrites lineage where the sidebar's drag may not.
	ParentID string `json:"parentId,omitempty"`

	// Order is this row's dense index within ParentID's sibling space, which
	// chats SHARE with AgentChatFolder rows: the two kinds interleave at every
	// level, so the panel merges both sets and sorts them on this one field.
	Order int `json:"order"`

	// Live turn state — folded from Turn events. Not durable truth: a crash
	// between the ledger append and the turn event can leave these stale; the
	// runner-exit reconcile (a dead CLI cannot still be mid-turn) repairs them.
	//
	// Working is the DERIVED answer to "is this chat busy", and it is deliberately
	// wider than "is a turn open":
	//
	//	Working = (CurrentTurnStarted != nil) || AsyncWork > 0
	//
	// because a turn ending is not the same fact as the agent being done. A CLI
	// that hands work to a BACKGROUND task genuinely ends its turn and goes quiet
	// waiting to be re-invoked when that task reports back — claude fires its Stop
	// hook right there. Folding Working from the turn alone therefore darkened the
	// spinner while the agent was still working, and the chat looked dead.
	Working            bool       `json:"working"`
	CurrentTurnStarted *time.Time `json:"currentTurnStarted,omitempty"`
	LastActivityAt     time.Time  `json:"lastActivityAt"`

	// AsyncWork is how many units of asynchronous work were still outstanding when
	// the last turn ENDED — the level the CLI itself reported on its turn_stop hook,
	// not a tally Crowbar keeps. Provider-agnostic: it is the SEMANTIC "async work is
	// in flight", never any one CLI's notion of a subagent, and a provider opts in by
	// naming the array to count in its descriptor. One that names nothing (codex)
	// never moves this off zero and keeps exactly its turn-only behaviour.
	//
	// IT CANNOT LEAK, and that is the entire reason it is a level rather than a
	// count of work_begin/work_end edges. Every turn_stop RESTATES it, so no stale
	// arithmetic survives a turn; a new turn zeroes it (a fresh turn supersedes any
	// claim the last one left behind); and the reconcile paths zero it outright,
	// since work announced by a CLI cannot outlive the CLI. There is no accumulator
	// to drift, so no sequence of hooks can strand the spinner ON — which a counter
	// demonstrably could, in both directions, because the edge hooks it would count
	// do not balance (see engine/agent.CanonicalEvent.AsyncWork for the measurements).
	AsyncWork int `json:"asyncWork"`

	// LedgerCursor is the count of ledger entries the aggregate has observed —
	// the pointer relating aggregate state to the append-only content log.
	LedgerCursor int `json:"ledgerCursor"`
}
