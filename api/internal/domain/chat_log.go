package domain

import "time"

// LedgerTurn is one entry in a chat's ledger: who spoke, under which provider and
// runner, and what was said.
//
// It is distinct from ActivityTurn, which is the live activity stream a running
// CLI emits. The ledger is what a chat REMEMBERS — it outlives every runner that
// wrote to it, which is why a turn carries the runner and session it came from
// rather than pointing at one.
type LedgerTurn struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Provider string `json:"provider"`

	RunnerID string `json:"runnerId,omitempty"`

	SessionID string    `json:"sessionId,omitempty"`
	Text      string    `json:"text"`
	Effort    string    `json:"effort,omitempty"`
	At        time.Time `json:"at"`
}

// LedgerMessage is a LedgerTurn at its position in the chat's ordering.
type LedgerMessage struct {
	Sequence int
	LedgerTurn
}

// LedgerPage is one window over a chat's ledger, with the cursors needed to walk
// further back.
type LedgerPage struct {
	Cursor       int
	OldestCursor int
	HasMore      bool
	Items        []LedgerMessage
}
