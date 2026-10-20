package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// errNoTurnToSteer is a steered prompt whose carrier is running no turn by
// the time it is recorded: the ordinary path delivers it instead.
var errNoTurnToSteer = errors.New("agent: steer: no turn is running on the carrier")

// submitPromptSteered delivers text into live's RUNNING turn when its
// descriptor declares steering: the prompt is recorded as steered and the
// turn's own end hook carries it into the same process, so nothing is quit or
// respawned. handled=false leaves the ordinary path to decide.
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
	dispatchText, err := rs.rewritePromptTextForDispatch(ctx, chat.WorkspaceID, chat.ID, text)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: materialize attachments: %w", err)
	}
	// The dispatch text rides on the row: the turn's end hook hands the CLI
	// what the ledger holds, so a failed commit leaves nothing behind.
	err = rs.recordPrompt(ctx, chat.ID, clientRequestID, text, dispatchText, ledger.DeliverySteer, live.ID,
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

// requireTurnOn admits a steered prompt only while runnerID runs the chat's
// turn: decided in the same commit as the turn's end, so either that end
// delivers the prompt or the prompt is never recorded as steered.
func requireTurnOn(runnerID string) func(ledger.State) error {
	return func(st ledger.State) error {
		if turn, ok := st.RunningTurn(); ok && turn.RunnerID == runnerID {
			return nil
		}
		return errNoTurnToSteer
	}
}
