package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *Usecase) recordTurn(
	ctx context.Context,
	chat domain.AgentChat,
	providerID, runnerID, sessionID string,
	role, text, effort string,
) error {
	if text == "" {
		return nil
	}
	return u.activity.AppendTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     turnID(ctx),
		Role:       role,
		ProviderID: providerID,
		RunnerID:   runnerID,
		SessionID:  sessionID,
		Text:       text,
		Effort:     effort,
		Now:        time.Now(),
	})
}

func turnID(ctx context.Context) string {
	if id := hookDeliveryID(ctx); id != "" {
		return id
	}
	return uuid.NewString()
}

func (u *Usecase) chatTurns(ctx context.Context, chatID string) ([]chatlog.Turn, error) {
	rows, err := u.activity.Turns(ctx, chatID, 0, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("agent: chat turns: %w", err)
	}
	return toChatTurns(rows), nil
}

func toChatTurns(rows []domain.ActivityTurn) []chatlog.Turn {
	out := make([]chatlog.Turn, 0, len(rows))
	for _, r := range rows {
		out = append(out, chatlog.Turn{
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

func (u *Usecase) chatPage(
	ctx context.Context,
	chatID string,
	after, before, limit int,
) (chatlog.Page, error) {
	if after > 0 && before > 0 {
		return chatlog.Page{}, fmt.Errorf(
			"agent: chat page: after and before are mutually exclusive: %w", apperr.ErrInvalidArgument,
		)
	}
	if limit <= 0 {
		return chatlog.Page{}, fmt.Errorf(
			"agent: chat page: limit must be positive: %w", apperr.ErrInvalidArgument,
		)
	}

	if after > 0 {
		rows, err := u.activity.Turns(ctx, chatID, int64(after), 0, limit+1)
		if err != nil {
			return chatlog.Page{}, fmt.Errorf("agent: chat page: %w", err)
		}
		hasMore := len(rows) > limit
		if hasMore {
			rows = rows[:limit]
		}
		return page(rows, hasMore), nil
	}

	rows, err := u.activity.TurnsBefore(ctx, chatID, int64(before), limit+1)
	if err != nil {
		return chatlog.Page{}, fmt.Errorf("agent: chat page: %w", err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[len(rows)-limit:]
	}
	return page(rows, hasMore), nil
}

func page(rows []domain.ActivityTurn, hasMore bool) chatlog.Page {
	out := chatlog.Page{HasMore: hasMore, Items: make([]chatlog.Message, 0, len(rows))}
	turns := toChatTurns(rows)
	for i, t := range turns {
		out.Items = append(out.Items, chatlog.Message{Sequence: int(rows[i].Seq), Turn: t})
	}
	if len(out.Items) > 0 {
		out.OldestCursor = out.Items[0].Sequence
		out.Cursor = out.Items[len(out.Items)-1].Sequence
	}
	return out
}

func (u *Usecase) renderConversation(
	ctx context.Context,
	chatID string,
	cut time.Time,
) ([]byte, error) {
	var (
		rows []domain.ActivityTurn
		err  error
	)
	if cut.IsZero() {
		rows, err = u.activity.Turns(ctx, chatID, 0, 0, 0)
	} else {
		rows, err = u.activity.TurnsSince(ctx, chatID, cut)
	}
	if err != nil {
		return nil, fmt.Errorf("agent: render conversation: %w", err)
	}
	var b strings.Builder
	for _, t := range toChatTurns(rows) {
		b.WriteString(t.Speaker())
		b.WriteString(": ")
		b.WriteString(t.Text)
		b.WriteString("\n\n")
	}
	return []byte(b.String()), nil
}
