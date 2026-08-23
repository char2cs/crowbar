package chatlog

import (
	"fmt"
	"time"
)

type Turn struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Provider string `json:"provider"`

	RunnerID string `json:"runnerId,omitempty"`

	SessionID string    `json:"sessionId,omitempty"`
	Text      string    `json:"text"`
	Effort    string    `json:"effort,omitempty"`
	At        time.Time `json:"at"`
}

func (t Turn) Speaker() string {
	switch t.Role {
	case "assistant":
		if t.Provider != "" {
			return fmt.Sprintf("assistant (%s)", t.Provider)
		}
	case "harness":
		if t.Provider != "" {
			return fmt.Sprintf("%s harness (injected, NOT the user)", t.Provider)
		}
		return "harness (injected, NOT the user)"
	}

	return t.Role
}

type Message struct {
	Sequence int
	Turn
}

type Page struct {
	Cursor       int
	OldestCursor int
	HasMore      bool
	Items        []Message
}
