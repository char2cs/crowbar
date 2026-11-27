package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ResolveConflicts struct {
	ID  string
	Now time.Time
}

func (c ResolveConflicts) AggregateID() string {
	return c.ID
}

func (c ResolveConflicts) EventName() string {
	return "workspace.conflictsResolved." + c.ID
}

func (c ResolveConflicts) ShouldSnapshot() bool {
	return false
}

func (c ResolveConflicts) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("resolve conflicts: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ResolveConflicts) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.LastActivity = c.Now
	if ws.Status == domain.WorkspaceStatusPRConflicts {
		ws.Status = domain.WorkspaceStatusNew
		if ws.PRUrl != "" {
			ws.Status = domain.WorkspaceStatusPROpen
		}
	}
	return ws
}
