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
	Resumable bool
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
	// A conversation with no opening time would stamp FirstSeenAt zero and drop
	// history ordering back onto insertion order.
	if c.Now.IsZero() {
		return fmt.Errorf("bind session: missing timestamp: %w", asynxModels.ErrValidation)
	}
	return nil
}

// EmitEvent binds the conversation AND stamps when it opened. Now is carried
// onto the aggregate (never dropped): the conversation projection reads it for
// FirstSeenAt, and the runner's own StartedAt cannot stand in for it — a runner
// binds its conversation after it spawns, sometimes hours after.
func (c BindSession) EmitEvent(current *domain.AgentRunner) domain.AgentRunner {
	next := *current
	next.CurrentSession = c.SessionID
	next.CurrentSessionSince = c.Now
	next.CurrentSessionResumable = c.Resumable
	return next
}
