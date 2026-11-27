package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type SetLock struct {
	ID        string
	Locked    *bool
	Protected bool
}

func (c SetLock) AggregateID() string {
	return c.ID
}

func (c SetLock) EventName() string {
	return "workspace.lock_set." + c.ID
}

func (c SetLock) ShouldSnapshot() bool {
	return true
}

func (c SetLock) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set lock: %w", asynxModels.ErrValidation)
	}
	if current.Kind == domain.WorkspaceKindHome {
		return refusedLock("The project's home checkout has no branch to lock.")
	}
	if current.Provisioning == domain.WorkspacePlaceholder {
		return refusedLock("This branch has no worktree of its own, so it can't be locked or unlocked.")
	}
	return nil
}

func refusedLock(why string) error {
	return &problem.Error{Code: problem.Unprocessable, Message: why, Err: asynxModels.ErrValidation}
}

func (c SetLock) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.LockOverride = c.Locked
	ws.Status = nextLockStatus(ws.Status, c.Locked, c.Protected)
	return ws
}
