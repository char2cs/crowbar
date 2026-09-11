package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
)

func (rs *Runners) PendingDelivery(ctx context.Context, chatID string) (termwait.Delivery, bool) {
	dir, err := rs.promptJournalDirFor(ctx, chatID)
	if err != nil {
		return termwait.Delivery{}, false
	}
	record, found, err := rs.prompts.ActiveDelivery(dir)
	if err != nil || !found {
		return termwait.Delivery{}, false
	}
	return termwait.Delivery{
		RequestID: record.RequestID,
		RunnerID:  record.RunnerID,
		CreatedAt: record.CreatedAt,
	}, true
}

// SettleDelivery retires a delivery that has gone quiet without ever producing
// a turn — the terminal-wait sweep's generic timeout, which is the ONLY caller
// and has no evidence of any kind that the provider took the prompt.
//
// consumed=false on the broadcast is therefore load-bearing, not a detail: the
// client's pending queue item is the only place the user's typed text is still
// RECOVERABLE at this point. This journal stores the literal text too, but
// Settle retires the record into PromptStateSettled — a proven-over outcome
// PendingPrompt deliberately never surfaces back to a client (pendingprompt.go)
// — and nothing reached the ledger, so a client told merely "this is over"
// deletes the words for good. See settleDelivery.
func (rs *Runners) SettleDelivery(ctx context.Context, chatID, requestID string) (bool, error) {
	return rs.settleDelivery(ctx, chatID, requestID, false)
}

// settleDelivery retires requestID and announces it, saying whether anything
// actually PROVED the provider took the prompt.
//
// consumed distinguishes the two callers, which the record's own state cannot:
// by the time either runs, the ledger has been reconciled and a prompt with a
// turn to show for it is already accepted (and so not retired here at all), so
// every delivery that reaches the broadcast looks identically evidence-free on
// disk. What separates them is WHO asked — a provider built-in that demonstrably
// ran (compact_post, via SettleDeliveryFor) versus a screen that simply went
// quiet for thirty seconds — and only the call site knows.
func (rs *Runners) settleDelivery(
	ctx context.Context, chatID, requestID string, consumed bool,
) (bool, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: chat: %w", err)
	}
	if err := rs.ReconcilePendingPromptFromLedger(ctx, chat); err != nil {
		return false, fmt.Errorf("agent: settle prompt delivery: ledger evidence: %w", err)
	}
	dir, err := rs.promptJournalDirFor(ctx, chatID)
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
		"chat_id", chatID, "client_request_id", requestID, "consumed", consumed)
	if rs.promptSettled != nil {
		rs.promptSettled(chatID, chat.WorkspaceID, requestID, consumed)
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
	// consumed=true: this caller runs off a signal that the CLI actually ACTED on
	// the prompt (compact_post follows the compaction the prompt asked for), so
	// the absent ledger turn is this delivery working as designed, not lost work.
	_, err := rs.settleDelivery(ctx, chatID, delivery.RequestID, true)
	return err
}

func (rs *Runners) promptJournalDirFor(ctx context.Context, chatID string) (string, error) {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: prompt journal dir: chat: %w", err)
	}
	chatsDir, err := rs.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return "", fmt.Errorf("agent: prompt journal dir: chats dir: %w", err)
	}
	return rs.prompts.Dir(chatsDir, chat.ID), nil
}
