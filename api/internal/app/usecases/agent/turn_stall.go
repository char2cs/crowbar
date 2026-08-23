package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (u *turnUsecase) closeStalledTurn(ctx context.Context, stall termwait.Stall) {
	if stall.ChatID == "" {
		return
	}

	working, known, _ := u.work.observe(stall.ChatID)
	if !known || !working {
		return
	}

	if err := u.activity.Abandon(ctx, stall.ChatID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: abandon conversation record",
			"chat_id", stall.ChatID, "err", err)
	}
	u.recordStallNotice(ctx, stall)

	abandoned, err := u.chats.AbandonTurn(ctx, stall.ChatID, time.Now())
	switch {
	case err == nil:
		u.work.set(stall.ChatID, abandoned.Working)
	case errors.Is(err, asynxModels.ErrValidation):

		u.work.set(stall.ChatID, false)
	default:
		slog.WarnContext(ctx, "agent: close stalled turn: abandon turn",
			"chat_id", stall.ChatID, "err", err)
		return
	}

	slog.InfoContext(ctx, "agent: closed a turn its provider abandoned",
		"chat_id", stall.ChatID, "provider", stall.ProviderID,
		"runner_id", stall.RunnerID, "notice", stall.Notice.Kind)
}

func (u *turnUsecase) recordStallNotice(ctx context.Context, stall termwait.Stall) {
	if stall.Notice.Text == "" {
		return
	}
	chat, err := u.chats.GetChat(ctx, stall.ChatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: read chat for notice",
			"chat_id", stall.ChatID, "err", err)
		return
	}
	if err := u.chat.recordTurn(
		ctx, chat,
		stall.ProviderID, stall.RunnerID, stall.SessionID,
		domain.TurnRoleNotice, stall.Notice.Text, "",
	); err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: record notice",
			"chat_id", stall.ChatID, "err", err)
	}
}

func (u *turnUsecase) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	home, err := u.home()
	if err != nil {
		return engineagents.TerminalNotice{}, false
	}
	descriptor, err := u.agents.Get(ctx, home, providerID)
	if err != nil {
		return engineagents.TerminalNotice{}, false
	}
	matcher, ok := descriptor.(engineagents.NoticeMatcher)
	if !ok {
		return engineagents.TerminalNotice{}, false
	}
	return matcher.MatchTerminalNotice(screen)
}

func (u *turnUsecase) OpenWork(ctx context.Context, chatID string) (bool, error) {
	tools, err := u.activity.ToolCalls(ctx, chatID, 0, 0)
	if err != nil {
		return false, err
	}
	for _, t := range tools {
		if t.Status == domain.ToolStatusRunning {
			return true, nil
		}
	}
	subagents, err := u.activity.Subagents(ctx, chatID)
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
