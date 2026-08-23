package commands

import (
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
)

// Exit tombstones a runner whose PTY has died. It is emitted ONLY by the
// terminal engine's exit callback or by boot reconciliation — i.e. it is always
// CAUSED by the PTY, never by an independent opinion about liveness. That is how
// the PTY stays the sole authority (spec §2).
type Exit struct {
	RunnerID string
	Now      time.Time
}

func (c Exit) AggregateID() string  { return c.RunnerID }
func (c Exit) EventName() string    { return "agentrunner.exited." + c.RunnerID }
func (c Exit) ShouldSnapshot() bool { return false }

func (c Exit) Validate(current *agents.Runner) error {
	if current == nil {
		return fmt.Errorf("exit runner: no runner: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Exit) EmitEvent(current *agents.Runner) agents.Runner {
	next := *current
	at := c.Now
	next.ExitedAt = &at
	return next
}
