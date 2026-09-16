package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetWorkspace writes the workspace a chat belongs to, filling the slot after
// creation (chats start with WorkspaceID optional). A bubble gaining worktree
// ownership and a promoted thread both use this command to establish workspace
// membership.
type SetWorkspace struct {
	ID          string
	WorkspaceID string
}

func (c SetWorkspace) AggregateID() string {
	return c.ID
}

func (c SetWorkspace) EventName() string {
	return "agentchat.workspace_set." + c.ID
}

func (c SetWorkspace) ShouldSnapshot() bool {
	return false
}

func (c SetWorkspace) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("set workspace: no chat: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" {
		return fmt.Errorf("set workspace: empty id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetWorkspace) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.WorkspaceID = c.WorkspaceID
	return chat
}
