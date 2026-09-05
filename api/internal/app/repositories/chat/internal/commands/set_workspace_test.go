package commands_test

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var _ asynxModels.Command[domain.Chat] = commands.SetWorkspace{}

func TestSetWorkspace_EmitEvent_SetsWorkspaceID(t *testing.T) {
	chat := &domain.Chat{ID: "chat-1"}
	cmd := commands.SetWorkspace{ID: "chat-1", WorkspaceID: "ws-1"}
	updated := cmd.EmitEvent(chat)
	assert.Equal(t, "ws-1", updated.WorkspaceID)
}

func TestSetWorkspace_Validate_RejectsEmptyID(t *testing.T) {
	cmd := commands.SetWorkspace{ID: "", WorkspaceID: "ws-1"}
	err := cmd.Validate(&domain.Chat{ID: "some-id"})
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestSetWorkspace_Validate_RefusesAChatThatDoesNotExist(t *testing.T) {
	cmd := commands.SetWorkspace{ID: "chat-1", WorkspaceID: "ws-1"}
	err := cmd.Validate(nil)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestSetWorkspace_IsRoutedAndNamedPerAggregate(t *testing.T) {
	cmd := commands.SetWorkspace{ID: "chat-1", WorkspaceID: "ws-1"}
	assert.Equal(t, "chat-1", cmd.AggregateID())
	assert.Equal(t, "agentchat.workspace_set.chat-1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}

func TestSetWorkspace_PreservesOtherFields(t *testing.T) {
	chat := &domain.Chat{
		ID:          "chat-1",
		WorkspaceID: "old-ws",
		ParentID:    "p1",
		Title:       "Original",
		TitleLocked: true,
		Order:       5,
	}
	cmd := commands.SetWorkspace{ID: "chat-1", WorkspaceID: "ws-2"}
	updated := cmd.EmitEvent(chat)

	assert.Equal(t, "ws-2", updated.WorkspaceID)
	assert.Equal(t, "p1", updated.ParentID)
	assert.Equal(t, "Original", updated.Title)
	assert.True(t, updated.TitleLocked)
	assert.Equal(t, 5, updated.Order)
}
