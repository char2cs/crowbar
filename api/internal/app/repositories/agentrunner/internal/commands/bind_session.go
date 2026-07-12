package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BindSession records the provider's conversation id for a runner that is
// staying put. It is the reducer's "bound" outcome: the runner announced its
// FIRST conversation. A runner that announces a DIFFERENT conversation is
// Move-ing, not binding.
type BindSession struct {
	RunnerID  string
	SessionID string
	Now       time.Time
}

func (c BindSession) AggregateID() string  { return c.RunnerID }
func (c BindSession) EventName() string    { return "agentrunner.session_bound." + c.RunnerID }
func (c BindSession) ShouldSnapshot() bool { return false }

func (c BindSession) Validate(current *domain.AgentRunner) error {
	if current == nil {
		return fmt.Errorf("bind session: no runner: %w", asynxModels.ErrValidation)
	}
	if c.SessionID == "" {
		return fmt.Errorf("bind session: missing session id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c BindSession) EmitEvent(current *domain.AgentRunner) domain.AgentRunner {
	next := *current
	next.CurrentSession = c.SessionID
	return next
}
