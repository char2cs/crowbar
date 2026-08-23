package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
)

func (u *runnerUsecase) PendingDelivery(ctx context.Context, chatID string) (termwait.Delivery, bool) {
	dir, err := u.promptJournalDirFor(ctx, chatID)
	if err != nil {
		return termwait.Delivery{}, false
	}
	record, found, err := u.prompts.ActiveDelivery(dir)
	if err != nil || !found {
		return termwait.Delivery{}, false
	}
	return termwait.Delivery{RequestID: record.RequestID, RunnerID: record.RunnerID}, true
}

func (u *runnerUsecase) SettleDelivery(ctx context.Context, chatID, requestID string) (bool, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: chat: %w", err)
	}
	if err := u.reconcilePendingPromptFromLedger(ctx, chat); err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: ledger evidence: %w", err)
	}
	dir, err := u.promptJournalDirFor(ctx, chatID)
	if err != nil {
		return false, err
	}
	retired, err := u.prompts.Settle(dir, requestID, time.Now())
	if err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: persist: %w", err)
	}
	if !retired {
		return false, nil
	}
	slog.InfoContext(ctx, "agent: prompt delivery produced no turn and was settled",
		"chat_id", chatID, "client_request_id", requestID)
	if u.promptSettled != nil {
		u.promptSettled(chatID, chat.WorkspaceID, requestID)
	}
	return true, nil
}

func (u *runnerUsecase) promptJournalDirFor(ctx context.Context, chatID string) (string, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: prompt journal dir: chat: %w", err)
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: prompt journal dir: chats dir: %w", err)
	}
	return u.prompts.Dir(chatsDir, chat.ID), nil
}
