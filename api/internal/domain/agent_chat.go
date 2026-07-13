package domain

import "time"

// AgentChat is the Crowbar-owned agentic conversation thread. Mutated only
// through asynx commands. Conversation content lives in the ledger, not here —
// this aggregate holds identity, title, live turn state, and a ledger cursor.
//
// It knows NOTHING about processes. `Segments []AgentSegment` and
// `ActiveSegmentID` are gone, and nothing replaces them on the aggregate (spec
// §2): a chat does not own the CLI that happens to be talking to it. The runner
// points at the chat, never the reverse, so "is this chat live?" is a QUERY
// against the runner read model (agentrunner.LiveRunnerForChat — a row exists
// exactly while its PTY does), never a flag stored here that could contradict
// the process. Which conversations a chat has hosted is likewise a PROJECTION of
// runner events (agentrunner's append-only chat_conversations), not chat state —
// so a conversation switch writes ONE aggregate (the runner) and the chat being
// left is never written to at all. That is what makes the torn cross-aggregate
// write that bricked a chat unrepresentable.
type AgentChat struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Title       string    `json:"title"`
	TitleLocked bool      `json:"titleLocked"`
	CreatedAt   time.Time `json:"createdAt"`

	// Live turn state — folded from Turn events. Not durable truth: a crash
	// between the ledger append and the turn event can leave these stale; the
	// runner-exit reconcile (a dead CLI cannot still be mid-turn) repairs them.
	Working            bool       `json:"working"`
	CurrentTurnStarted *time.Time `json:"currentTurnStarted,omitempty"`
	LastActivityAt     time.Time  `json:"lastActivityAt"`

	// LedgerCursor is the count of ledger entries the aggregate has observed —
	// the pointer relating aggregate state to the append-only content log.
	LedgerCursor int `json:"ledgerCursor"`
}
