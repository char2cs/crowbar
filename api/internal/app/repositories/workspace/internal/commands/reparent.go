package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Reparent re-points a workspace at a new parent and records the new fork point
// (07 §4). The leaf-guard lives in the usecase; the command only mutates fields.
type Reparent struct {
	ID           string
	ParentID     string
	ForkPointSha string
	Now          time.Time
}

func (c Reparent) AggregateID() string {
	return c.ID
}

func (c Reparent) EventName() string {
	return "workspace.reparented." + c.ID
}

func (c Reparent) ShouldSnapshot() bool {
	return true
}

func (c Reparent) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("reparent: %w", asynxModels.ErrValidation)
	}
	if c.ParentID == "" || c.ForkPointSha == "" {
		return fmt.Errorf("reparent: missing parent or fork point: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Reparent) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.ParentID = c.ParentID
	ws.ForkPointSha = c.ForkPointSha
	ws.LastActivity = c.Now
	return ws
}
