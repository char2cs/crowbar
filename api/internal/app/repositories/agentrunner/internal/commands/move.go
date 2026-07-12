package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Move repoints a runner at a different chat and conversation. This ONE command
// is the entire reason the refactor exists.
//
// Note what it does NOT do: it does not touch the chat being left, and it does
// not touch the chat being entered. It cannot fail on their state, because it
// never reads their state. That is what makes the torn cross-aggregate write
// (which bricked a chat in production) unrepresentable rather than merely
// avoided — there is no second write to fail.
//
// The PTY, the provider and the runner id all travel unchanged: the process did
// not restart, it just changed which conversation it is showing. This is why the
// terminal never remounts on a /clear.
type Move struct {
	RunnerID  string
	ToChatID  string
	SessionID string
	Now       time.Time
}

func (c Move) AggregateID() string  { return c.RunnerID }
func (c Move) EventName() string    { return "agentrunner.moved." + c.RunnerID }
func (c Move) ShouldSnapshot() bool { return false }

func (c Move) Validate(current *domain.AgentRunner) error {
	if current == nil {
		return fmt.Errorf("move runner: no runner: %w", asynxModels.ErrValidation)
	}
	if c.ToChatID == "" {
		return fmt.Errorf("move runner: missing chat id: %w", asynxModels.ErrValidation)
	}
	if c.SessionID == "" {
		return fmt.Errorf("move runner: missing session id: %w", asynxModels.ErrValidation)
	}
	return nil
}

// EmitEvent repoints the runner AND stamps when the conversation it entered
// opened. Now is carried onto the aggregate (never dropped): without it the
// conversation projection would stamp this conversation with the runner's spawn
// time, which for a runner that moves is arbitrarily old — and two runners
// writing into one chat would then order their conversations by whose PROCESS is
// older rather than by which CONVERSATION is newer.
func (c Move) EmitEvent(current *domain.AgentRunner) domain.AgentRunner {
	next := *current
	next.CurrentChatID = c.ToChatID
	next.CurrentSession = c.SessionID
	next.CurrentSessionSince = c.Now
	return next
}
