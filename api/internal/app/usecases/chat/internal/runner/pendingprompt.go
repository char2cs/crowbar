package runner

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// PendingPrompt returns the chat's most recent prompt submission, if it is
// still in a state where the frontend might have lost its own copy but the
// prompt has not been proven to have failed or landed: dispatching, spawned
// or uncertain. Accepted means the ledger already has the reply's own
// request — the client's own transcript read already shows it. Failed and
// settled are both proven-over outcomes, and recovering either would hand
// the client a row that can never be dispatched or resolved (settled
// especially: a provider built-in like /compact settles with no ledger turn
// to ever confirm it, so a recovered settled row would wedge the FIFO head
// forever). A record with no stored text is a pre-migration record, written
// before PromptRequest gained a Text field: also nothing to recover.
func (rs *Runners) PendingPrompt(
	ctx context.Context,
	chatID string,
) (domain.PendingPrompt, bool, error) {
	dir, err := rs.promptJournalDirFor(ctx, chatID)
	if err != nil {
		return domain.PendingPrompt{}, false, err
	}
	record, found, err := rs.prompts.LatestRequest(dir)
	if err != nil || !found {
		return domain.PendingPrompt{}, false, err
	}
	switch record.State {
	case agentjournal.PromptStateDispatching,
		agentjournal.PromptStateSpawned,
		agentjournal.PromptStateUncertain:
	default:
		return domain.PendingPrompt{}, false, nil
	}
	if record.Text == "" {
		return domain.PendingPrompt{}, false, nil
	}
	return domain.PendingPrompt{
		Text:      record.Text,
		State:     record.State,
		RequestID: record.RequestID,
	}, true, nil
}
