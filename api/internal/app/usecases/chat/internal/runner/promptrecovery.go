package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Crash recovery for the at-most-once submission journal.
//
// Crowbar can die between writing the dispatch intent and confirming the runner
// that took it. Everything here answers the one question that leaves behind: given
// the journal record and what the chat's ledger now shows, did the provider accept
// that prompt or not? A wrong answer in either direction is visible to the user —
// a silent duplicate, or a submission refused forever.

func (rs *Runners) classifyPriorAttempt(
	ctx context.Context,
	chat domain.Chat,
	journalDir, clientRequestID string,
	existing agentjournal.PromptRequest,
) (domain.AgentPromptSubmission, bool, error) {
	if existing.RunnerID != "" && existing.TerminalSessionID != "" &&
		(existing.State == agentjournal.PromptStateSpawned || existing.State == agentjournal.PromptStateAccepted) {
		return promptSubmission(existing), true, nil
	}
	if existing.State == agentjournal.PromptStateDispatching ||
		existing.State == agentjournal.PromptStateSpawned ||
		existing.State == agentjournal.PromptStateUncertain {
		return rs.recoverPriorDelivery(ctx, chat, journalDir, clientRequestID, existing)
	}
	if existing.State == agentjournal.PromptStateAccepted {
		return domain.AgentPromptSubmission{}, true, ErrPromptAlreadyAccepted
	}
	return domain.AgentPromptSubmission{}, false, nil
}

func (rs *Runners) recoverPriorDelivery(
	ctx context.Context,
	chat domain.Chat,
	journalDir, clientRequestID string,
	existing agentjournal.PromptRequest,
) (domain.AgentPromptSubmission, bool, error) {
	accepted, err := rs.promptRecordAccepted(ctx, chat, existing)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, rs.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "recover prior delivery", err,
		)
	}
	if !accepted {
		return domain.AgentPromptSubmission{}, true, ErrPromptOutcomeUnknown
	}
	if _, err := rs.prompts.MarkAccepted(journalDir, clientRequestID, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: submit prompt: persist recovered acceptance",
			"chat_id", chat.ID, "client_request_id", clientRequestID, "err", err)
	}
	return domain.AgentPromptSubmission{}, true, ErrPromptAlreadyAccepted
}

func (rs *Runners) promptRecordAccepted(
	ctx context.Context,
	chat domain.Chat,
	record agentjournal.PromptRequest,
) (bool, error) {
	turns, err := rs.conversations.ChatTurns(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: recover prompt request: turns: %w", err)
	}
	for _, t := range turns {
		if deliveredThisRequest(t, record) && agentjournal.PromptTextHash(t.Text) == record.TextHash {
			return true, nil
		}
	}
	return false, nil
}

func deliveredThisRequest(
	turn domain.LedgerTurn,
	record agentjournal.PromptRequest,
) bool {
	if turn.Role != "user" || turn.Provider != record.ProviderID || turn.At.Before(record.CreatedAt) {
		return false
	}
	if record.RunnerID != "" {
		return turn.RunnerID == record.RunnerID
	}
	return turn.RunnerID != "" && turn.RunnerID != record.OutgoingRunnerID
}

func (rs *Runners) ReconcilePendingPromptFromLedger(
	ctx context.Context,
	chat domain.Chat,
) error {
	dir, err := rs.promptJournalDirFor(chat.ID)
	if err != nil {
		return fmt.Errorf("reconcile prompt acceptance: journal dir: %w", err)
	}
	record, found, err := rs.prompts.ActiveDelivery(dir)
	if err != nil || !found {
		return err
	}
	accepted, err := rs.promptRecordAccepted(ctx, chat, record)
	if err != nil || !accepted {
		return err
	}
	if _, err := rs.prompts.MarkAccepted(dir, record.RequestID, time.Now()); err != nil {
		return fmt.Errorf("reconcile prompt acceptance: persist: %w", err)
	}
	return nil
}

func (rs *Runners) ConfirmPromptAccepted(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	text string,
) error {
	dir, err := rs.promptJournalDirFor(chat.ID)
	if err != nil {
		return fmt.Errorf("prompt journal dir: %w", err)
	}
	return rs.prompts.ConfirmAccepted(
		dir, runner.ID, runner.ProviderID, agentjournal.PromptTextHash(text), time.Now(),
	)
}

func (rs *Runners) reconcilePromptRunnerDeparture(
	ctx context.Context,
	runner engineagents.Runner,
	chatID string,
) {
	if chatID == "" {
		return
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return
	}
	dir, err := rs.promptJournalDirFor(chat.ID)
	if err != nil {
		return
	}
	record, found, err := rs.prompts.ActiveForRunner(dir, runner.ID, runner.ProviderID)
	if err != nil || !found {
		return
	}
	accepted, err := rs.promptRecordAccepted(ctx, chat, record)
	if err != nil {
		return
	}
	if accepted {
		_, _ = rs.prompts.MarkAccepted(dir, record.RequestID, time.Now())
		return
	}

	_ = rs.prompts.MarkUncertain(dir, record.RequestID, time.Now())
}
