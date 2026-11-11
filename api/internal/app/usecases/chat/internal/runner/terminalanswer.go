package runner

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// AnswerTerminalWait types the descriptor-declared reply to the modal the chat's
// CLI is parked on. It does not clear the wait: the sweep does that once the
// screen no longer shows the prompt.
func (rs *Runners) AnswerTerminalWait(ctx context.Context, chatID, answerID string) error {
	wait := rs.TerminalWait(chatID)
	if !wait.Waiting {
		return fmt.Errorf("%w: the provider is no longer waiting on a prompt", problem.ErrConflict)
	}
	var keys string
	var found bool
	for _, a := range wait.Answers {
		if a.ID == answerID {
			keys, found = a.Keys, true
		}
	}
	if !found {
		return fmt.Errorf("%w: this prompt has no answer %q", problem.ErrInvalidArgument, answerID)
	}

	live, ok := rs.live.byChat(chatID)
	if !ok {
		return fmt.Errorf("%w: the provider is no longer running", problem.ErrConflict)
	}
	if err := rs.term.Write(ctx, live.TerminalSession, []byte(keys)); err != nil {
		return fmt.Errorf("agent: answer terminal wait: write: %w", err)
	}
	return nil
}
