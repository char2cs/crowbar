package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// The message page bounds ReadMessages serves under: a caller that asks for
// nothing gets a screenful, and one that asks for everything is refused rather
// than being allowed to pull an entire conversation into one response.
const (
	defaultMessagePageLimit = 100
	maxMessagePageLimit     = 200
)

// ChatTurns is every message of the chat, in the order it is shown.
func (c *Conversations) ChatTurns(ctx context.Context, chatID string) ([]domain.LedgerTurn, error) {
	turns, err := c.transcript.ChatTurns(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: chat turns: %w", err)
	}
	return turns, nil
}

func (c *Conversations) renderConversation(
	ctx context.Context,
	chatID string,
	cut time.Time,
) ([]byte, error) {
	turns, err := c.ChatTurns(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: render conversation: %w", err)
	}
	since := turns[:0:0]
	for _, t := range turns {
		if t.At.After(cut) {
			since = append(since, t)
		}
	}
	// Capped to the same "recent" window get_chat_log itself pages against: a
	// handoff is injected cold into the next provider's context on every switch,
	// so an ever-growing chat must not mean an ever-growing preamble.
	kept, note := agenttools.RecentHandoffWindow(chatID, since)
	var b strings.Builder
	b.WriteString(note)
	for _, t := range kept {
		b.WriteString(speaker(t))
		b.WriteString(": ")
		b.WriteString(t.Text)
		b.WriteString("\n\n")
	}
	return []byte(b.String()), nil
}

// appendNote adds text to the chat's conversation as a note from Crowbar, for
// the agent to read.
func (c *Conversations) appendNote(ctx context.Context, chatID, text string) error {
	row, err := ledger.NewRow(ledger.KindUserMessage, uuid.NewString(), "",
		ledger.UserMessage{Text: text, Origin: ledger.OriginNote})
	if err != nil {
		return fmt.Errorf("agent: append note: %w", err)
	}
	if _, _, err := c.ledger.Append(ctx, chatID, row); err != nil {
		return fmt.Errorf("agent: append note: %w", err)
	}
	return nil
}

func (c *Conversations) ReadMessages(
	ctx context.Context,
	chatID string,
	after, before, limit int,
) (domain.LedgerPage, error) {
	if after < 0 || before < 0 || (after > 0 && before > 0) {
		return domain.LedgerPage{}, fmt.Errorf("agent: read messages: invalid cursor: %w", apperr.ErrInvalidArgument)
	}
	if limit == 0 {
		limit = defaultMessagePageLimit
	}
	if limit < 1 || limit > maxMessagePageLimit {
		return domain.LedgerPage{}, fmt.Errorf("agent: read messages: limit must be between 1 and %d: %w", maxMessagePageLimit, apperr.ErrInvalidArgument)
	}
	if _, err := c.chats.GetChat(ctx, chatID); err != nil {
		return domain.LedgerPage{}, fmt.Errorf("agent: read messages: chat: %w", err)
	}
	page, err := c.transcript.Messages(ctx, chatID, after, before, limit)
	if err != nil {
		return domain.LedgerPage{}, fmt.Errorf("agent: read messages: %w", err)
	}
	return page, nil
}

// speaker names who produced a ledger turn, for the two places this package
// RENDERS one: the handoff preamble and the chat log.
//
// It is here and not on domain.LedgerTurn because it is display text, not a fact
// about the turn — "harness (injected, NOT the user)" exists to stop a reader (and
// a model reading its own handoff) mistaking an injected prompt for something a
// person typed.
func speaker(t domain.LedgerTurn) string {
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
