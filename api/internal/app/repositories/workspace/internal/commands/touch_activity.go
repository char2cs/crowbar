package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TouchActivity bumps only lastActivity, representing chat/agent activity (01 §5).
type TouchActivity struct {
	ID  string
	Now time.Time
}

func (c TouchActivity) AggregateID() string {
	return c.ID
}

func (c TouchActivity) EventName() string {
	return "workspace.activity_touched." + c.ID
}

func (c TouchActivity) ShouldSnapshot() bool {
	return false
}

func (c TouchActivity) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("touch activity: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c TouchActivity) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.LastActivity = c.Now
	return ws
}
