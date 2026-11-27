package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var errNoTurnToSteer = errors.New("agent: steer: no turn is running on the carrier")

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
	if _, viaAPI := rs.apiConns.get(live.ID); viaAPI {
		return domain.AgentPromptSubmission{}, false, nil
	}
	if st, err := rs.ledger.State(ctx, chat.ID); err != nil || requireTurnOn(live.ID)(st) != nil {
		return domain.AgentPromptSubmission{}, err != nil, err
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
	dispatchText, err := rs.rewritePromptTextForDispatch(chat.ID, text)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: materialize attachments: %w", err)
	}
	_, err = rs.recordPrompt(ctx, chat.ID, clientRequestID, text, dispatchText, ledger.DeliverySteer, live.ID,
		requireTurnOn(live.ID))
	if errors.Is(err, errNoTurnToSteer) {
		return domain.AgentPromptSubmission{}, false, nil
	}
	if err != nil {
		return domain.AgentPromptSubmission{}, true, err
	}
	submission, err := rs.commitPromptSpawn(ctx, chat.ID, clientRequestID, live.ID)
	return submission, true, err
}

func requireTurnOn(runnerID string) func(ledger.State) error {
	return func(st ledger.State) error {
		if turn, ok := st.RunningTurn(); ok && turn.RunnerID == runnerID {
			return nil
		}
		return errNoTurnToSteer
	}
}
