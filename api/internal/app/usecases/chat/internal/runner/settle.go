package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

func (rs *Runners) PendingDelivery(ctx context.Context, chatID string) (termwait.Delivery, bool) {
	dir, err := rs.promptJournalDirFor(chatID)
	if err != nil {
		return termwait.Delivery{}, false
	}
	record, found, err := rs.prompts.ActiveDelivery(dir)
	if err != nil || !found {
		return termwait.Delivery{}, false
	}
	return termwait.Delivery{RequestID: record.RequestID, RunnerID: record.RunnerID}, true
}

func (rs *Runners) SettleDelivery(ctx context.Context, chatID, requestID string) (bool, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: chat: %w", err)
	}
	if err := rs.ReconcilePendingPromptFromLedger(ctx, chat); err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: ledger evidence: %w", err)
	}
	dir, err := rs.promptJournalDirFor(chatID)
	if err != nil {
		return false, err
	}
	retired, err := rs.prompts.Settle(dir, requestID, time.Now())
	if err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: persist: %w", err)
	}
	if !retired {
		return false, nil
	}
	slog.InfoContext(ctx, "agent: prompt delivery produced no turn and was settled",
		"chat_id", chatID, "client_request_id", requestID)
	if rs.promptSettled != nil {
		rs.promptSettled(chatID, chat.WorkspaceID, requestID)
	}
	return true, nil
}

// SettleDeliveryFor retires runnerID's pending prompt delivery on chatID right
// now, if it has one — used when some OTHER signal already proves the CLI
// acted and is done, so nothing should keep waiting out the full
// deliveryQuiet timeout before releasing the composer's "sending" state.
//
// compact_post is the case this exists for: `/compact` is delivered as an
// ordinary prompt (compact.go), but a compaction never confirms via a
// user_prompt hook or produces a ledger turn — the ONLY thing that would
// otherwise ever settle it is termwait's generic 30s quiet timeout, even
// though the compaction itself (its own compact_pre/compact_post pair) is
// typically done within seconds. A no-op if nothing is pending, or if what
// is pending belongs to a different runner.
func (rs *Runners) SettleDeliveryFor(ctx context.Context, chatID, runnerID string) error {
	delivery, ok := rs.PendingDelivery(ctx, chatID)
	if !ok || delivery.RunnerID != runnerID {
		return nil
	}
	_, err := rs.SettleDelivery(ctx, chatID, delivery.RequestID)
	return err
}

// promptJournalDirFor returns the on-disk directory backing chatID's
// at-most-once prompt-delivery journal.
//
// It is keyed by the chat's own id alone, never by a workspace lookup:
// WorkspaceID is optional and mutable — a bubble chat has none until promoted,
// and even a set workspace's own directory can move later (WorktreePath
// transitions blocked -> provisioned) — so a derivation built from it would
// either error while the field is empty or move the journal out from under a
// chat still writing to it (spec §1.5). worktreepath.LedgerChatsDir is the one
// anchor neither can perturb.
func (rs *Runners) promptJournalDirFor(chatID string) (string, error) {
	home, err := rs.home()
	if err != nil {
		return "", fmt.Errorf("agent: prompt journal dir: crowbar home: %w", err)
	}
	return rs.prompts.Dir(worktreepath.LedgerChatsDir(home), chatID), nil
}
