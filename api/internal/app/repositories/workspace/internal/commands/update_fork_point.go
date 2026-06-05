package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// UpdateForkPoint resets a kept child's forkPointSha to the parent's post-merge
// tip after a local merge, for every strategy (07 §3.1).
type UpdateForkPoint struct {
	ID           string
	ForkPointSha string
}

func (c UpdateForkPoint) AggregateID() string {
	return c.ID
}

func (c UpdateForkPoint) EventName() string {
	return "workspace.fork_point_updated." + c.ID
}

func (c UpdateForkPoint) ShouldSnapshot() bool {
	return false
}

func (c UpdateForkPoint) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("update fork point: %w", asynxModels.ErrValidation)
	}
	if c.ForkPointSha == "" {
		return fmt.Errorf("update fork point: missing sha: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c UpdateForkPoint) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.ForkPointSha = c.ForkPointSha
	return ws
}
