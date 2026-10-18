// Package ledger is a chat's append-only record: every fact about a
// conversation is a Row, and a chat's State is the fold of its rows.
package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Kind names what a row records; it selects the shape of its body.
type Kind string

const (
	KindUserMessage   Kind = "user_message"
	KindTurnStarted   Kind = "turn_started"
	KindPromptSettled Kind = "prompt_settled"
)

// ErrMalformedRow is a row the ledger refuses to record.
var ErrMalformedRow = errors.New("ledger: malformed row")

// Row is the envelope every ledger fact travels in.
type Row struct {
	ChatID string          `json:"chatId"`
	Seq    int64           `json:"seq"`              // assigned by the store, 1-based, contiguous per chat
	Kind   Kind            `json:"kind"`             // selects Body's shape
	TurnID string          `json:"turnId,omitempty"` // Crowbar-minted (uuid v4); "" for chat-level rows
	Scope  string          `json:"scope,omitempty"`  // "" = main agent, else a subagent scope id
	At     time.Time       `json:"at"`               // stamped by the store's clock
	Body   json.RawMessage `json:"body"`
}

// NewRow encodes body into a row of kind; the store assigns chat, seq and time.
func NewRow[B any](kind Kind, turnID, scope string, body B) (Row, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return Row{}, fmt.Errorf("ledger: new row: %s: %w", kind, err)
	}
	return Row{Kind: kind, TurnID: turnID, Scope: scope, Body: raw}, nil
}

// DecodeBody decodes r's body as B.
func DecodeBody[B any](r Row) (B, error) {
	var body B
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return body, fmt.Errorf("ledger: decode %s body: %w", r.Kind, err)
	}
	return body, nil
}

// Check refuses only malformed rows: an unknown kind, a turn-scoped kind
// without a turn, an undecodable body. A row the State has moved past is
// late, not malformed, and Apply ignores it.
func Check(_ State, r Row) error {
	switch r.Kind {
	case KindUserMessage:
		return checkTurnRow[UserMessage](r)
	case KindTurnStarted:
		return checkTurnRow[TurnStarted](r)
	case KindPromptSettled:
		return checkBody[PromptSettled](r)
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrMalformedRow, r.Kind)
	}
}

func checkTurnRow[B any](r Row) error {
	if r.TurnID == "" {
		return fmt.Errorf("%w: %s without a turn id", ErrMalformedRow, r.Kind)
	}
	return checkBody[B](r)
}

func checkBody[B any](r Row) error {
	if _, err := DecodeBody[B](r); err != nil {
		return fmt.Errorf("%w: %w", ErrMalformedRow, err)
	}
	return nil
}
