package commands_test

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var (
	_ asynxModels.Command[domain.AgentChat] = commands.SetPlacement{}
	_ asynxModels.Command[domain.AgentChat] = commands.SetOrder{}
)

func TestSetPlacement_WritesBothHalvesOfThePlacement(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", ParentID: "old", Order: 3}

	out := commands.SetPlacement{ID: "c1", ParentID: "p1", Order: 1}.EmitEvent(chat)

	assert.Equal(t, "p1", out.ParentID)
	assert.Equal(t, 1, out.Order)
}

// The panel root is a real destination, so an empty parent is a move OUT and not
// a missing field.
func TestSetPlacement_AnEmptyParentIsTheRoot(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", ParentID: "p1"}

	require.NoError(t, commands.SetPlacement{ID: "c1", ParentID: ""}.Validate(chat))
	assert.Empty(t, commands.SetPlacement{ID: "c1", ParentID: ""}.EmitEvent(chat).ParentID)
}

func TestSetPlacement_RefusesAChatUnderItself(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1"}

	err := commands.SetPlacement{ID: "c1", ParentID: "c1"}.Validate(chat)

	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestSetPlacement_RefusesANegativeOrder(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1"}

	assert.ErrorIs(t,
		commands.SetPlacement{ID: "c1", Order: -1}.Validate(chat), asynxModels.ErrValidation)
}

func TestSetPlacement_RefusesAChatThatDoesNotExist(t *testing.T) {
	assert.ErrorIs(t, commands.SetPlacement{ID: "c1"}.Validate(nil), asynxModels.ErrValidation)
}

func TestSetPlacement_IsRoutedAndNamedPerAggregate(t *testing.T) {
	cmd := commands.SetPlacement{ID: "c1"}

	assert.Equal(t, "c1", cmd.AggregateID())
	assert.Equal(t, "agentchat.placement_set.c1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}

// The property the whole command exists for: it must be unable to move a chat,
// whatever the caller believed the parent to be when it planned the renumber.
func TestSetOrder_LeavesTheParentExactlyAsTheLogHasIt(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", ParentID: "p1", Order: 0}

	out := commands.SetOrder{ID: "c1", Order: 2}.EmitEvent(chat)

	assert.Equal(t, "p1", out.ParentID, "a renumber decides nothing about where a chat lives")
	assert.Equal(t, 2, out.Order)
}

func TestSetOrder_LeavesAChatAtTheRootAtTheRoot(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Order: 4}

	assert.Empty(t, commands.SetOrder{ID: "c1", Order: 0}.EmitEvent(chat).ParentID)
}

// Everything else about the chat is untouched: this is a renumber, not a save of
// a row the caller read a moment ago.
func TestSetOrder_TouchesNothingButTheIndex(t *testing.T) {
	chat := &domain.AgentChat{
		ID: "c1", WorkspaceID: "ws-1", ParentID: "p1", Title: "Pricing", TitleLocked: true,
	}

	out := commands.SetOrder{ID: "c1", Order: 1}.EmitEvent(chat)

	assert.Equal(t, "ws-1", out.WorkspaceID)
	assert.Equal(t, "Pricing", out.Title)
	assert.True(t, out.TitleLocked)
}

func TestSetOrder_RefusesANegativeOrder(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1"}

	assert.ErrorIs(t, commands.SetOrder{ID: "c1", Order: -1}.Validate(chat), asynxModels.ErrValidation)
}

func TestSetOrder_RefusesAChatThatDoesNotExist(t *testing.T) {
	assert.ErrorIs(t, commands.SetOrder{ID: "c1"}.Validate(nil), asynxModels.ErrValidation)
}

func TestSetOrder_AcceptsTheFirstSlot(t *testing.T) {
	assert.NoError(t, commands.SetOrder{ID: "c1", Order: 0}.Validate(&domain.AgentChat{ID: "c1"}))
}

// A distinct event name from SetPlacement, so a renumber and a move are not the
// same entry in a log that is read back to explain what happened to a chat.
func TestSetOrder_IsRoutedAndNamedPerAggregate(t *testing.T) {
	cmd := commands.SetOrder{ID: "c1"}

	assert.Equal(t, "c1", cmd.AggregateID())
	assert.Equal(t, "agentchat.order_set.c1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}
