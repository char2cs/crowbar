package runner

import (
	"cmp"
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func (rs *Runners) TerminalWait(chatID string) domain.AgentTerminalWait {
	if rs.termWait == nil {
		return domain.AgentTerminalWait{}
	}
	return rs.termWait.Wait(chatID)
}

// StartTerminalWaitSweep starts the screen sweep, when the terminal seam can
// render a screen.
func (rs *Runners) StartTerminalWaitSweep(ctx context.Context) {
	if rs.termWait == nil {
		return
	}
	// The verdict rides the chat snapshot: a change republishes it.
	rs.termWait.Run(ctx, func(chatID, _ string, wait domain.AgentTerminalWait) {
		if wait.Waiting && wait.Kind == "" {
			rs.settleCommandsShown(ctx, chatID)
		}
		rs.touch(ctx, chatID)
	})
}

// settleCommandsShown settles the commands chatID's runner carries as taken: a
// dialog over a pending command is that command running in the CLI, and no hook
// reports it, nor its closing.
func (rs *Runners) settleCommandsShown(ctx context.Context, chatID string) {
	runner, err := rs.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return
	}
	command := rs.commandOf(ctx, runner)
	if command == nil {
		return
	}
	rs.settlePrompts(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Settle(st, func(p ledger.Prompt) bool {
			return p.RunnerID == runner.ID && command(cmp.Or(p.Dispatch, p.Text))
		}, true, ledger.ReasonProviderBuiltin)
	})
}
