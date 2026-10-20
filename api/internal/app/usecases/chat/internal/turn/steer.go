package turn

import (
	"cmp"
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// pendingSteer is the steered prompt runnerID carries on its running turn.
func pendingSteer(st ledger.State, runnerID string) (ledger.Prompt, bool) {
	for _, p := range st.PendingPrompts() {
		if p.Delivery == ledger.DeliverySteer && p.RunnerID == runnerID {
			return p, true
		}
	}
	return ledger.Prompt{}, false
}

// settleSteer settles the steered prompt runnerID carries for reason.
func settleSteer(st ledger.State, runnerID, reason string) ([]ledger.Row, error) {
	return ledger.Settle(st, func(p ledger.Prompt) bool {
		return p.Delivery == ledger.DeliverySteer && p.RunnerID == runnerID
	}, false, reason)
}

// steerReply is the hook reply that hands p to the CLI as its turn ends, or
// false when the hook's channel cannot carry one. The CLI is handed the text
// the prompt's row records for dispatch, never a copy held in memory.
func steerReply(ctx context.Context, p ledger.Prompt, agent engineagents.Agent) ([]byte, bool) {
	stdout, ok := agent.PromptSteer(cmp.Or(p.Dispatch, p.Text))
	return []byte(stdout), ok && hookChannel(ctx) == engineagents.ChannelHooks
}

// recordSteered records a delivered steered prompt in the conversation record
// as the user turn the CLI's continuation answers.
func (t *Turns) recordSteered(ctx context.Context, chat domain.Chat, runner engineagents.Runner, p ledger.Prompt) {
	err := t.conversations.AppendRunnerTurn(
		ctx, chat, p.RequestID, runner.ProviderID, runner.ID,
		t.boundSession(ctx, chat.ID, runner.ProviderID), domain.TurnRoleUser, p.Text,
	)
	t.openAssistantTurn(ctx, chat, runner)
	note(ctx, "record steered prompt", err)
}
