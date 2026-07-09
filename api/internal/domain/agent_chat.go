package domain

import "time"

// AgentChat is the Crowbar-owned agentic conversation aggregate, tracked across
// provider segments. Mutated only through asynx commands. Conversation content
// lives in the ledger, not here — this aggregate holds identity, segments,
// session ids, title, and live Working state, plus a ledger cursor.
type AgentChat struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	Title       string          `json:"title"`
	TitleLocked bool            `json:"titleLocked"`
	CreatedAt   time.Time       `json:"createdAt"`
	Status      AgentChatStatus `json:"status,omitempty"`

	// gorm:"-" keeps this embedded slice out of the legacy gorm store's
	// AutoMigrate (it can't map a slice-of-struct and errors at container boot);
	// the asynx read model serializes the whole aggregate as JSON and ignores
	// gorm tags, so this is invisible there. Remove once the legacy store is gone.
	Segments        []AgentSegment `json:"segments" gorm:"-"`
	ActiveSegmentID string         `json:"activeSegmentId,omitempty"`

	// Live turn state — folded from Turn events, reconciled on boot. Not durable
	// truth: a crash between the ledger append and the turn event can leave these
	// stale; the boot reactor repairs them.
	Working            bool       `json:"working"`
	CurrentTurnStarted *time.Time `json:"currentTurnStarted,omitempty"`
	LastActivityAt     time.Time  `json:"lastActivityAt"`

	// LedgerCursor is the count of ledger entries the aggregate has observed —
	// the pointer relating aggregate state to the append-only content log.
	LedgerCursor int `json:"ledgerCursor"`
}
