package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// beginLiveDelivery opens the journal entry for a prompt delivered to live
// ITSELF — over its api connection or into its running turn — rather than to a
// replacement. done=false means a new attempt began and the caller delivers it.
func (rs *Runners) beginLiveDelivery(
	ctx context.Context,
	chat domain.Chat,
	journalDir, clientRequestID, textHash string,
	live engineagents.Runner,
	text string,
) (domain.AgentPromptSubmission, bool, error) {
	prior, existingAttempt, err := rs.prompts.Begin(
		journalDir, clientRequestID, text, textHash, live.ProviderID, live.ID, live.ID, time.Now(),
	)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf(
			"agent: submit prompt: begin durable dispatch: %w", promptJournalError(err),
		)
	}
	if !existingAttempt {
		return domain.AgentPromptSubmission{}, false, nil
	}
	result, done, classifyErr := rs.classifyPriorAttempt(ctx, chat, journalDir, clientRequestID, prior)
	if done {
		return result, true, classifyErr
	}
	return domain.AgentPromptSubmission{}, true, ErrPromptOutcomeUnknown
}

// submitPromptSteered delivers text into live's RUNNING turn when its
// descriptor declares steering: the message is parked on the turn and the
// turn's own end hook carries it into the same process, so nothing is quit or
// respawned. handled=false leaves the ordinary path to decide — idle chats
// restart as always, and a chat whose turn is not in flight (background work,
// a changed model) stays busy.
func (rs *Runners) submitPromptSteered(
	ctx context.Context,
	chat domain.Chat,
	journalDir, clientRequestID, textHash string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
	text string,
) (domain.AgentPromptSubmission, bool, error) {
	if _, steerable := descriptor.PromptSteer(text); !steerable {
		return domain.AgentPromptSubmission{}, false, nil
	}
	if _, viaAPI := rs.apiConns.get(live.ID); viaAPI || len(rs.inflightTurns.Inflight(chat.ID)) == 0 {
		return domain.AgentPromptSubmission{}, false, nil
	}
	restart, err := rs.selectionRequiresRestart(ctx, chat.ID, live, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf(
			"agent: submit prompt: check selection restart: %w", err,
		)
	}
	if restart {
		return domain.AgentPromptSubmission{}, false, nil
	}
	if submission, done, beginErr := rs.beginLiveDelivery(
		ctx, chat, journalDir, clientRequestID, textHash, live, text,
	); done {
		return submission, true, beginErr
	}
	dispatchText, err := rs.rewritePromptTextForDispatch(ctx, chat.WorkspaceID, chat.ID, text)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, rs.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "materialize attachments", err,
		)
	}
	parked := rs.turns.QueueSteer(live.ID, inflight.Steered{
		ChatID: chat.ID, RequestID: clientRequestID, Text: text, DispatchText: dispatchText,
	})
	if !parked {
		_ = rs.prompts.MarkFailedDispatch(journalDir, clientRequestID, time.Now())
		return domain.AgentPromptSubmission{}, true, ErrPromptBusy
	}
	submission, err := rs.commitPromptSpawn(ctx, journalDir, clientRequestID, textHash, live.ID)
	return submission, true, err
}

// RefuseSteered fails a parked prompt whose turn ended without delivering it,
// and tells the client it must keep its own copy of the text.
func (rs *Runners) RefuseSteered(ctx context.Context, s inflight.Steered) {
	dir, err := rs.promptJournalDirFor(s.ChatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: refuse steered prompt: journal dir", "chat_id", s.ChatID, "err", err)
		return
	}
	if err := rs.prompts.MarkRefused(dir, s.RequestID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: refuse steered prompt: persist",
			"chat_id", s.ChatID, "client_request_id", s.RequestID, "err", err)
		return
	}
	chat, err := rs.chats.GetChat(ctx, s.ChatID)
	if err != nil || rs.promptSettled == nil {
		return
	}
	rs.promptSettled(s.ChatID, chat.WorkspaceID, s.RequestID, false)
}
