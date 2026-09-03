package turn

import (
	"context"
	"errors"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (t *Turns) CloseStalledTurn(ctx context.Context, stall seam.Stall) {
	if stall.ChatID == "" {
		return
	}

	working, known, _ := t.work.Observe(stall.ChatID)
	if !known || !working {
		return
	}

	if err := t.activity.Abandon(ctx, stall.ChatID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: abandon conversation record",
			"chat_id", stall.ChatID, "err", err)
	}
	t.recordStallNotice(ctx, stall)

	abandoned, err := t.chats.AbandonTurn(ctx, stall.ChatID, time.Now())
	switch {
	case err == nil:
		t.work.Set(stall.ChatID, abandoned.Working)
	case errors.Is(err, asynxModels.ErrValidation):

		t.work.Set(stall.ChatID, false)
	default:
		slog.WarnContext(ctx, "agent: close stalled turn: abandon turn",
			"chat_id", stall.ChatID, "err", err)
		return
	}

	slog.InfoContext(ctx, "agent: closed a turn its provider abandoned",
		"chat_id", stall.ChatID, "provider", stall.ProviderID,
		"runner_id", stall.RunnerID, "notice", stall.Notice.Kind)
}

func (t *Turns) recordStallNotice(ctx context.Context, stall seam.Stall) {
	if stall.Notice.Text == "" {
		return
	}
	chat, err := t.chats.GetChat(ctx, stall.ChatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: read chat for notice",
			"chat_id", stall.ChatID, "err", err)
		return
	}
	if err := t.conversations.RecordTurn(
		ctx, chat,
		stall.ProviderID, stall.RunnerID, stall.SessionID,
		domain.TurnRoleNotice, stall.Notice.Text, "",
	); err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: record notice",
			"chat_id", stall.ChatID, "err", err)
	}
}

func (t *Turns) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	home, err := t.home()
	if err != nil {
		return engineagents.TerminalNotice{}, false
	}
	descriptor, err := t.agents.Get(ctx, home, providerID)
	if err != nil {
		return engineagents.TerminalNotice{}, false
	}
	matcher, ok := descriptor.(engineagents.NoticeMatcher)
	if !ok {
		return engineagents.TerminalNotice{}, false
	}
	return matcher.MatchTerminalNotice(screen)
}

func (t *Turns) OpenWork(ctx context.Context, chatID string) (bool, error) {
	tools, err := t.activity.ToolCalls(ctx, chatID, 0, 0)
	if err != nil {
		return false, err
	}
	for _, t := range tools {
		if t.Status == domain.ToolStatusRunning {
			return true, nil
		}
	}
	subagents, err := t.activity.Subagents(ctx, chatID)
	if err != nil {
		return false, err
	}
	for _, s := range subagents {
		if s.EndedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

// MatchTerminalPrompt asks a provider's descriptor whether a rendered screen is
// one of the modal prompts it declares — a trust dialog, a permission gate.
//
// It is silent rather than an error for every unresolvable input: an unknown
// provider, an unreadable home, a screen that matches nothing. The caller is a
// sweep over live terminals, and a sweep that failed on a screen it could not
// classify would stop classifying the ones it could.
func (t *Turns) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	home, err := t.home()
	if err != nil {
		return engineagents.TerminalPrompt{}, false
	}
	descriptor, err := t.agents.Get(ctx, home, providerID)
	if err != nil {
		return engineagents.TerminalPrompt{}, false
	}
	return descriptor.MatchTerminalPrompt(screen)
}
