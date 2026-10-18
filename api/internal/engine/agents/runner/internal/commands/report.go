package commands

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	asynxModels "github.com/char2cs/asynx/models"
)

// Report records the model and effort the CLI said it resolved to at session
// start, which no CLI exposes afterwards. Display only: LaunchModel/LaunchEffort
// stay what Crowbar asked for. Its frame keeps the kind the client already
// reads as non-structural.
type Report struct {
	RunnerID string
	Model    string
	Effort   string
}

func (c Report) AggregateID() string  { return c.RunnerID }
func (c Report) EventName() string    { return "runner.session_bound." + c.RunnerID }
func (c Report) ShouldSnapshot() bool { return false }

func (c Report) Validate(current *agents.Runner) error {
	if current == nil {
		return fmt.Errorf("report: no runner: %w", asynxModels.ErrValidation)
	}
	if c.Model == "" && c.Effort == "" {
		return fmt.Errorf("report: nothing reported: %w", asynxModels.ErrValidation)
	}
	return nil
}

// EmitEvent leaves a value the CLI did not report as it was.
func (c Report) EmitEvent(current *agents.Runner) agents.Runner {
	next := *current
	if c.Model != "" {
		next.ReportedModel = c.Model
	}
	if c.Effort != "" {
		next.ReportedEffort = c.Effort
	}
	return next
}
