package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
)

// PendingDelivery implements termwait.Deliveries. It reports the chat's one open
// prompt delivery, and nothing at all when the journal cannot be read: a delivery
// that cannot be seen is not one to conclude anything about.
func (u *Usecase) PendingDelivery(ctx context.Context, chatID string) (termwait.Delivery, bool) {
	dir, err := u.promptJournalDirFor(ctx, chatID)
	if err != nil {
		return termwait.Delivery{}, false
	}
	record, found, err := u.prompts.activeDelivery(dir)
	if err != nil || !found {
		return termwait.Delivery{}, false
	}
	return termwait.Delivery{RequestID: record.RequestID, RunnerID: record.RunnerID}, true
}

// SettleDelivery implements termwait.Deliveries: the CLI this prompt was handed to
// has come to rest without producing a turn, so the record stops being a barrier.
//
// The ledger is consulted FIRST, and it wins. A prompt hook that did arrive is
// better evidence than a still screen about the same delivery, and the two can
// legitimately race — the hook landing while this tick was deciding. Recording the
// weaker conclusion over the stronger one would leave the journal saying Crowbar
// guessed at something a provider had actually confirmed.
func (u *Usecase) SettleDelivery(ctx context.Context, chatID, requestID string) (bool, error) {
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
	retired, err := u.prompts.settle(dir, requestID, time.Now())
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

func (u *Usecase) promptJournalDirFor(ctx context.Context, chatID string) (string, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: prompt journal dir: chat: %w", err)
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: prompt journal dir: chats dir: %w", err)
	}
	return promptJournalDir(filepath.Join(chatsDir, chat.ID)), nil
}
