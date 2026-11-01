package domain

import "time"

// LedgerTurn is one message of a chat: who spoke, under which provider and
// runner, and what was said. It outlives every runner that wrote it, so it
// names the runner and session rather than pointing at one.
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
