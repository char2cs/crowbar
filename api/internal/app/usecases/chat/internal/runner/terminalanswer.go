package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// AnswerTerminalWait types the descriptor-declared reply to the modal the chat's
// CLI is parked on. It does not clear the wait: the sweep does that once the
// screen no longer shows the prompt.
func (rs *Runners) AnswerTerminalWait(ctx context.Context, chatID, answerID string) error {
	wait := rs.TerminalWait(chatID)
	if !wait.Waiting {
		return fmt.Errorf("%w: the provider is no longer waiting on a prompt", apperr.ErrConflict)
	}
	var keys string
	var found bool
	for _, a := range wait.Answers {
		if a.ID == answerID {
			keys, found = a.Keys, true
		}
	}
	if !found {
		return fmt.Errorf("%w: this prompt has no answer %q", apperr.ErrInvalidArgument, answerID)
	}

	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return fmt.Errorf("%w: the provider is no longer running", apperr.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("agent: answer terminal wait: live runner: %w", err)
	}
	if err := rs.term.Write(ctx, live.TerminalSession, []byte(keys)); err != nil {
		return fmt.Errorf("agent: answer terminal wait: write: %w", err)
	}
	return nil
}
