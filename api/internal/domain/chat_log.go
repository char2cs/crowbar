package domain

import "time"

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
