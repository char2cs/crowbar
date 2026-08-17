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

// recordTurn writes one conversation turn into the activity record.
//
// The turn id is the hook's DELIVERY id when there is one. That is not a
// convenience: the delivery id is the relay's durable idempotency key, and the
// projection upserts by id, so a hook redelivered after a lost response rewrites
// the same row instead of duplicating the turn. Without it, "the daemon did not
// answer in time" and "the user said it twice" would be indistinguishable.
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

// chatTurns returns a chat's whole conversation in order.
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

// chatPage reads a bounded window of a chat's messages.
//
//   - after > 0 returns the first limit messages after that sequence
//     (incremental refresh); HasMore reports further NEWER rows.
//   - before > 0 returns the newest limit messages below that sequence (paging
//     upward); HasMore reports further OLDER rows.
//   - with neither, it returns the newest limit messages and HasMore reports
//     whether older rows exist.
//
// The two cursors are mutually exclusive. The caller owns request validation, but
// an ambiguous request is still refused here so a non-HTTP consumer cannot
// silently receive a misleading page.
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
		// One extra row answers HasMore without a second query.
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

	// One extra row again answers HasMore — here, whether OLDER rows remain.
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

// renderConversation builds the handoff document: every turn, or only those after
// cut, one per line under its speaker.
//
// It is byte-compatible with the flat-file record it replaced. That matters more
// than it looks: this text is what a provider joining a chat is handed as context,
// and a change in its shape is a change in what every future handoff reads.
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
