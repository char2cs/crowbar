package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
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
	// `spawned` is the ONE state only MarkSpawned can produce, so it proves
	// this delivery committed on its own — which matters now that an
	// api-driven runner commits with an EMPTY terminal session (apirunner.go)
	// and a non-empty one can no longer stand in for the proof.
	//
	// `accepted` still needs that terminal session, and deliberately: the
	// user_prompt hook can advance a record straight out of `dispatching`,
	// before MarkSpawned has run at all, so an accepted record with no
	// committed identity has no delivery to replay and must recover from the
	// ledger instead — see TestSubmitPrompt_RunnerLookupFailureAndAcceptedCrashGapAreSafe.
	committed := existing.State == agentjournal.PromptStateSpawned ||
		(existing.State == agentjournal.PromptStateAccepted && existing.TerminalSessionID != "")
	if existing.RunnerID != "" && committed {
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
		if !deliveredThisRequest(t, record) {
			continue
		}
		// A specific expected runner id (record.RunnerID != "", the ordinary
		// case) is already a unique identity match on its own: a runner
		// delivers at MOST one user-prompt-opened turn in its whole life — it
		// is replaced wholesale by the next message's own spawn — so a "user"
		// turn under this exact runnerID can only be the one THIS dispatch
		// produced. The text hash adds nothing there except false negatives
		// whenever the ledger's stored text legitimately differs from the
		// journal's original-dispatch-text hash: attachments, a leading-sigil
		// escape, and (mergeLeadingPositional) an injected gap merged ahead of
		// a real prompt in the same positional all do this by design.
		// recordUserTurn (turn.go) already strips all three back out before
		// storing, but this identity check must not re-depend on that holding
		// perfectly forever the way it once silently did.
		//
		// Only the weaker no-runnerID fallback below — matching on
		// role/provider/timing alone, reached when the record predates
		// knowing which runner would deliver it — still needs the hash: on
		// its own it could otherwise land on some unrelated LATER message to
		// the same provider.
		if record.RunnerID != "" {
			return true, nil
		}
		if agentjournal.PromptTextHash(t.Text) == record.TextHash {
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
	rs.settlePromptRecord(ctx, chat, dir, record)
}

// settleDepartedPromptDelivery is the OWNERSHIP half of "is a delivery
// pending?", and the reason a wedged chat cannot happen again.
//
// A "spawned" record is a claim about a live process. The journal is a
// directory of files and cannot check one, so it answers from the state string
// alone — and nothing downgrades "spawned" on its own. That asymmetry with
// "dispatching" is deliberate and stays: a dispatching record can only have
// been written by a caller that is no longer running, whereas a spawned one may
// be a CLI answering right now, and downgrading that would let a second prompt
// through mid-turn.
//
// Which leaves exactly one way to be wrong, and it bricked a chat: the owner
// departed and nothing settled the record, so every resume and every prompt
// answered 409 conflict for the life of the chat. Enumerating departure sites is
// what already failed — three were wired, two were missed. Asked at the QUESTION
// instead: a record whose owner is not the chat's live runner is not in flight,
// whatever its state says.
func (rs *Runners) settleDepartedPromptDelivery(
	ctx context.Context,
	chat domain.Chat,
) {
	dir, err := rs.promptJournalDirFor(chat.ID)
	if err != nil {
		return
	}
	record, found, err := rs.prompts.ActiveDelivery(dir)
	if err != nil || !found {
		return
	}
	if record.State != agentjournal.PromptStateSpawned || record.RunnerID == "" {
		return
	}
	if rs.runnerStillOnChat(ctx, chat.ID, record.RunnerID) {
		return
	}
	slog.WarnContext(ctx, "agent: prompt delivery outlived the runner that owned it; settling",
		"chat_id", chat.ID, "client_request_id", record.RequestID, "runner_id", record.RunnerID)
	rs.settlePromptRecord(ctx, chat, dir, record)
}

// runnerStillOnChat is deliberately asymmetric about failure: only a runner the
// store positively reports as GONE settles a record. A read that merely failed
// is not evidence the owner left, and treating it as such would retire a
// delivery a live CLI is still answering.
func (rs *Runners) runnerStillOnChat(ctx context.Context, chatID, runnerID string) bool {
	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return false
	}
	if err != nil {
		return true
	}
	return live.ID == runnerID
}

// settlePromptRecord retires one in-flight record against the ledger: accepted
// when a turn proves the provider took it, uncertain otherwise. Uncertain
// blocks an automatic RETRY (at-most-once is preserved) without blocking the
// chat, which is what un-wedges it.
func (rs *Runners) settlePromptRecord(
	ctx context.Context,
	chat domain.Chat,
	dir string,
	record agentjournal.PromptRequest,
) {
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
