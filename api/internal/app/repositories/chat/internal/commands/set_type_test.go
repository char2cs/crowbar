package commands_test

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var _ asynxModels.Command[domain.Chat] = commands.SetType{}

func TestSetType_EmitEvent_RewritesTheKind(t *testing.T) {
	chat := &domain.Chat{ID: "chat-1", Type: domain.ChatTypeChat}
	cmd := commands.SetType{ID: "chat-1", Type: domain.ChatTypeBranch}
	assert.Equal(t, domain.ChatTypeBranch, cmd.EmitEvent(chat).Type)
}

// The retype is what keeps one workspace to one owning row when that workspace
// changes character, so everything the row IS besides its kind — its identity,
// its placement, its workspace, its title — has to come through untouched.
func TestSetType_PreservesEverythingButTheKind(t *testing.T) {
	chat := &domain.Chat{
		ID:              "chat-1",
		Type:            domain.ChatTypeChat,
		WorkspaceID:     "ws-1",
		ParentID:        "p1",
		Title:           "Original",
		TitleLocked:     true,
		Order:           5,
		PermissionLevel: "guarded",
		LedgerCursor:    12,
	}
	updated := commands.SetType{ID: "chat-1", Type: domain.ChatTypeBranch}.EmitEvent(chat)

	assert.Equal(t, domain.ChatTypeBranch, updated.Type)
	assert.Equal(t, "chat-1", updated.ID)
	assert.Equal(t, "ws-1", updated.WorkspaceID)
	assert.Equal(t, "p1", updated.ParentID)
	assert.Equal(t, "Original", updated.Title)
	assert.True(t, updated.TitleLocked)
	assert.Equal(t, 5, updated.Order)
	assert.Equal(t, "guarded", updated.PermissionLevel)
	assert.Equal(t, 12, updated.LedgerCursor)
}

func TestSetType_Validate_RefusesAChatThatDoesNotExist(t *testing.T) {
	cmd := commands.SetType{ID: "chat-1", Type: domain.ChatTypeBranch}
	assert.ErrorIs(t, cmd.Validate(nil), asynxModels.ErrValidation)
}

func TestSetType_Validate_RefusesAKindThatIsNotOne(t *testing.T) {
	cmd := commands.SetType{ID: "chat-1", Type: domain.ChatType("workspace")}
	err := cmd.Validate(&domain.Chat{ID: "chat-1", Type: domain.ChatTypeChat})
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

// Folder rows and chat rows share one table but not one set of verbs — a folder
// delete PROMOTES what it held where a chat delete CASCADES into it — so a
// retype across that line would hand the rows already filed under it the
// opposite rule from the one they were filed under. Refused in both directions.
func TestSetType_Validate_RefusesCrossingTheFolderBoundary(t *testing.T) {
	intoFolder := commands.SetType{ID: "chat-1", Type: domain.ChatTypeFolder}
	assert.ErrorIs(t,
		intoFolder.Validate(&domain.Chat{ID: "chat-1", Type: domain.ChatTypeChat}),
		asynxModels.ErrValidation)

	outOfFolder := commands.SetType{ID: "folder-1", Type: domain.ChatTypeBranch}
	assert.ErrorIs(t,
		outOfFolder.Validate(&domain.Chat{ID: "folder-1", Type: domain.ChatTypeFolder}),
		asynxModels.ErrValidation)
}

func TestSetType_Validate_AcceptsAChatBecomingABranchRow(t *testing.T) {
	cmd := commands.SetType{ID: "chat-1", Type: domain.ChatTypeBranch}
	assert.NoError(t, cmd.Validate(&domain.Chat{ID: "chat-1", Type: domain.ChatTypeChat}))
}

func TestSetType_IsRoutedAndNamedPerAggregate(t *testing.T) {
	cmd := commands.SetType{ID: "chat-1", Type: domain.ChatTypeBranch}
	assert.Equal(t, "chat-1", cmd.AggregateID())
	assert.Equal(t, "agentchat.type_set.chat-1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}
