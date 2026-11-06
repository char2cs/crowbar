package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func (c *Conversations) renderConversation(
	ctx context.Context,
	chatID string,
	cut time.Time,
) ([]byte, error) {
	turns, err := c.ChatTurns(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: render conversation: %w", err)
	}
	st, err := c.ledger.State(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: render conversation: %w", err)
	}
	// What the person cleared is no conversation to hand on.
	resuming := !cut.IsZero()
	if cleared := st.ClearedAt(); cleared.After(cut) {
		cut = cleared
	}
	since, pin := turns[:0:0], -1
	for _, t := range turns {
		if !t.At.After(cut) {
			continue
		}
		if pin < 0 && !resuming && t.Role == domain.TurnRoleUser {
			pin = len(since)
		}
		since = append(since, t)
	}
	// Capped like get_chat_log's recent window, as a handoff is injected cold on
	// every switch; a fresh join keeps the first ask, which a resume already holds.
	head, note, recent := agenttools.HandoffWindow(chatID, since, pin)
	var b strings.Builder
	writeTurns(&b, head)
	b.WriteString(note)
	writeTurns(&b, recent)
	return []byte(b.String()), nil
}

func writeTurns(b *strings.Builder, turns []domain.LedgerTurn) {
	for _, t := range turns {
		b.WriteString(speaker(t))
		b.WriteString(": ")
		b.WriteString(t.Text)
		b.WriteString("\n\n")
	}
}

// appendNote adds text to the chat's conversation as a note from Crowbar, for
// the agent to read.
func (c *Conversations) appendNote(ctx context.Context, chatID, text string) error {
	row, err := ledger.NewRow(ledger.KindUserMessage, uuid.NewString(), "",
		ledger.UserMessage{Text: text, Origin: ledger.OriginNote})
	if err != nil {
		return fmt.Errorf("agent: append note: %w", err)
	}
	_, err = c.writes.Do(ctx, chatID, func(ledger.State) ([]ledger.Row, error) { return []ledger.Row{row}, nil })
	if err != nil {
		return fmt.Errorf("agent: append note: %w", err)
	}
	return nil
}

// speaker names who produced a ledger turn, as display text for the handoff and chat
// log. "harness (injected, NOT the user)" stops a reader — or a model reading its own
// handoff — mistaking an injected prompt for something a person typed.
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
