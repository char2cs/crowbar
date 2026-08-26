package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The message page bounds ReadMessages serves under: a caller that asks for
// nothing gets a screenful, and one that asks for everything is refused rather
// than being allowed to pull an entire conversation into one response.
const (
	defaultMessagePageLimit = 100
	maxMessagePageLimit     = 200
)

func (c *Conversations) RecordTurn(
	ctx context.Context,
	chat domain.Chat,
	providerID, runnerID, sessionID string,
	role, text, effort string,
) error {
	if text == "" {
		return nil
	}
	return c.activity.AppendTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     inflight.RecordID(ctx),
		Role:       role,
		ProviderID: providerID,
		RunnerID:   runnerID,
		SessionID:  sessionID,
		Text:       text,
		Effort:     effort,
		Now:        time.Now(),
	})
}

func (c *Conversations) ChatTurns(ctx context.Context, chatID string) ([]domain.LedgerTurn, error) {
	rows, err := c.activity.Turns(ctx, chatID, 0, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("agent: chat turns: %w", err)
	}
	return toChatTurns(rows), nil
}

func toChatTurns(rows []domain.ActivityTurn) []domain.LedgerTurn {
	out := make([]domain.LedgerTurn, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.LedgerTurn{
			ID:        r.ID,
			Role:      r.Role,
			Provider:  r.ProviderID,
			RunnerID:  r.RunnerID,
			SessionID: r.SessionID,
			Text:      r.Text,
			Effort:    r.Effort,
			At:        r.StartedAt,
		})
	}
	return out
}

func (c *Conversations) chatPage(
	ctx context.Context,
	chatID string,
	after, before, limit int,
) (domain.LedgerPage, error) {
	if after > 0 && before > 0 {
		return domain.LedgerPage{}, fmt.Errorf(
			"agent: chat page: after and before are mutually exclusive: %w", apperr.ErrInvalidArgument,
		)
	}
	if limit <= 0 {
		return domain.LedgerPage{}, fmt.Errorf(
			"agent: chat page: limit must be positive: %w", apperr.ErrInvalidArgument,
		)
	}

	if after > 0 {
		rows, err := c.activity.Turns(ctx, chatID, int64(after), 0, limit+1)
		return trimFront(rows, limit, err)
	}
	rows, err := c.activity.TurnsBefore(ctx, chatID, int64(before), limit+1)
	return trimBack(rows, limit, err)
}

// trimFront and trimBack turn a limit+1 read into a page plus the has-more flag.
//
// Both read one row PAST the limit and drop it: that extra row is the only honest
// answer to "is there more", and asking the store for a count instead would be a
// second query racing the first. They differ only in which end the extra row
// arrives at — forward paging overshoots at the tail, backward paging at the head.
func trimFront(rows []domain.ActivityTurn, limit int, err error) (domain.LedgerPage, error) {
	if err != nil {
		return domain.LedgerPage{}, fmt.Errorf("agent: chat page: %w", err)
	}
	if hasMore := len(rows) > limit; hasMore {
		return page(rows[:limit], true), nil
	}
	return page(rows, false), nil
}

func trimBack(rows []domain.ActivityTurn, limit int, err error) (domain.LedgerPage, error) {
	if err != nil {
		return domain.LedgerPage{}, fmt.Errorf("agent: chat page: %w", err)
	}
	if hasMore := len(rows) > limit; hasMore {
		return page(rows[len(rows)-limit:], true), nil
	}
	return page(rows, false), nil
}

func page(rows []domain.ActivityTurn, hasMore bool) domain.LedgerPage {
	out := domain.LedgerPage{HasMore: hasMore, Items: make([]domain.LedgerMessage, 0, len(rows))}
	turns := toChatTurns(rows)
	for i, t := range turns {
		out.Items = append(out.Items, domain.LedgerMessage{Sequence: int(rows[i].Seq), LedgerTurn: t})
	}
	if len(out.Items) > 0 {
		out.OldestCursor = out.Items[0].Sequence
		out.Cursor = out.Items[len(out.Items)-1].Sequence
	}
	return out
}

func (c *Conversations) renderConversation(
	ctx context.Context,
	chatID string,
	cut time.Time,
) ([]byte, error) {
	var (
		rows []domain.ActivityTurn
		err  error
	)
	if cut.IsZero() {
		rows, err = c.activity.Turns(ctx, chatID, 0, 0, 0)
	} else {
		rows, err = c.activity.TurnsSince(ctx, chatID, cut)
	}
	if err != nil {
		return nil, fmt.Errorf("agent: render conversation: %w", err)
	}
	// Capped to the same "recent" window get_chat_log itself pages against: a
	// handoff is injected cold into the next provider's context on every switch,
	// so an ever-growing chat must not mean an ever-growing preamble.
	kept, note := agenttools.RecentHandoffWindow(chatID, rows)
	var b strings.Builder
	b.WriteString(note)
	for _, t := range toChatTurns(kept) {
		b.WriteString(speaker(t))
		b.WriteString(": ")
		b.WriteString(t.Text)
		b.WriteString("\n\n")
	}
	return []byte(b.String()), nil
}

func (c *Conversations) appendTurn(
	ctx context.Context,
	chat domain.Chat,
	providerID string,
	role, text string,
) error {
	return c.AppendRunnerTurn(ctx, chat, providerID, "", "", role, text)
}

func (c *Conversations) AppendRunnerTurn(
	ctx context.Context,
	chat domain.Chat,
	providerID, runnerID, sessionID string,
	role, text string,
) error {
	if err := c.RecordTurn(ctx, chat, providerID, runnerID, sessionID, role, text, ""); err != nil {
		return fmt.Errorf("agent: append turn: %w", err)
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
	page, err := c.chatPage(ctx, chatID, after, before, limit)
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
