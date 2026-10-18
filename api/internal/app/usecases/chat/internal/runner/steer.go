package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// submitPromptSteered delivers text into live's RUNNING turn when its
// descriptor declares steering: the message is parked on the turn and the
// turn's own end hook carries it into the same process, so nothing is quit or
// respawned. handled=false leaves the ordinary path to decide — idle chats
// restart as always, and a chat whose turn is not in flight (background work,
// a changed model) stays busy.
func (rs *Runners) submitPromptSteered(
	ctx context.Context,
	chat domain.Chat,
	clientRequestID string,
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
	dispatchText, err := rs.rewritePromptTextForDispatch(ctx, chat.WorkspaceID, chat.ID, text)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: materialize attachments: %w", err)
	}
	if err := rs.recordPrompt(ctx, chat.ID, clientRequestID, text, ledger.DeliverySteer, live.ID); err != nil {
		return domain.AgentPromptSubmission{}, true, err
	}
	parked := rs.turns.QueueSteer(live.ID, inflight.Steered{
		ChatID: chat.ID, RequestID: clientRequestID, Text: text, DispatchText: dispatchText,
	})
	if !parked {
		return domain.AgentPromptSubmission{}, true, rs.failPrompt(ctx, chat.ID, clientRequestID, ErrPromptBusy)
	}
	submission, err := rs.commitPromptSpawn(ctx, chat.ID, clientRequestID, live.ID)
	return submission, true, err
}

// RefuseSteered settles a parked prompt whose turn ended without delivering
// it, so the client keeps its own copy of the text.
func (rs *Runners) RefuseSteered(ctx context.Context, s inflight.Steered) {
	rs.settleUndelivered(ctx, s.ChatID, s.RequestID)
}
