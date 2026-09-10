package commands_test

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/node/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var (
	_ asynxModels.Command[domain.Node] = commands.SetPlacement{}
	_ asynxModels.Command[domain.Node] = commands.SetOrder{}
)

func TestSetPlacement_WritesBothHalvesOfThePlacement(t *testing.T) {
	n := &domain.Node{ID: "n1", ParentID: "old", Order: 3}

	out := commands.SetPlacement{ID: "n1", ParentID: "p1", Order: 1}.EmitEvent(n)

	assert.Equal(t, "p1", out.ParentID)
	assert.Equal(t, 1, out.Order)
}

// The panel root is a real destination, so an empty parent is a move OUT and not
// a missing field.
func TestSetPlacement_AnEmptyParentIsTheRoot(t *testing.T) {
	n := &domain.Node{ID: "n1", ParentID: "p1"}

	require.NoError(t, commands.SetPlacement{ID: "n1", ParentID: ""}.Validate(n))
	assert.Empty(t, commands.SetPlacement{ID: "n1", ParentID: ""}.EmitEvent(n).ParentID)
}

func TestSetPlacement_RefusesANodeUnderItself(t *testing.T) {
	n := &domain.Node{ID: "n1"}

	err := commands.SetPlacement{ID: "n1", ParentID: "n1"}.Validate(n)

	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestSetPlacement_RefusesANegativeOrder(t *testing.T) {
	n := &domain.Node{ID: "n1"}

	assert.ErrorIs(t,
		commands.SetPlacement{ID: "n1", Order: -1}.Validate(n), asynxModels.ErrValidation)
}

func TestSetPlacement_RefusesANodeThatDoesNotExist(t *testing.T) {
	assert.ErrorIs(t, commands.SetPlacement{ID: "n1"}.Validate(nil), asynxModels.ErrValidation)
}

func TestSetPlacement_IsRoutedAndNamedPerAggregate(t *testing.T) {
	cmd := commands.SetPlacement{ID: "n1"}

	assert.Equal(t, "n1", cmd.AggregateID())
	assert.Equal(t, "node.placement_set.n1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}

// The property the whole command exists for: it must be unable to move a node,
// whatever the caller believed the parent to be when it planned the renumber.
// This is the exact race a dragged repo silently snapping back to position 0
// was caused by — see 2026-09-08 sidebar-placement-unification design §2.2.
func TestSetOrder_LeavesTheParentExactlyAsTheLogHasIt(t *testing.T) {
	n := &domain.Node{ID: "n1", ParentID: "p1", Order: 0}

	out := commands.SetOrder{ID: "n1", Order: 2}.EmitEvent(n)

	assert.Equal(t, "p1", out.ParentID, "a renumber decides nothing about where a node lives")
	assert.Equal(t, 2, out.Order)
}

func TestSetOrder_LeavesANodeAtTheRootAtTheRoot(t *testing.T) {
	n := &domain.Node{ID: "n1", Order: 4}

	assert.Empty(t, commands.SetOrder{ID: "n1", Order: 0}.EmitEvent(n).ParentID)
}

// Everything else about the node is untouched: this is a renumber, not a save
// of a row the caller read a moment ago.
func TestSetOrder_TouchesNothingButTheIndex(t *testing.T) {
	n := &domain.Node{ID: "n1", Kind: domain.NodeKindWorkspace, ParentID: "p1"}

	out := commands.SetOrder{ID: "n1", Order: 1}.EmitEvent(n)

	assert.Equal(t, domain.NodeKindWorkspace, out.Kind)
	assert.Equal(t, "p1", out.ParentID)
}

func TestSetOrder_RefusesANegativeOrder(t *testing.T) {
	n := &domain.Node{ID: "n1"}

	assert.ErrorIs(t, commands.SetOrder{ID: "n1", Order: -1}.Validate(n), asynxModels.ErrValidation)
}

func TestSetOrder_RefusesANodeThatDoesNotExist(t *testing.T) {
	assert.ErrorIs(t, commands.SetOrder{ID: "n1"}.Validate(nil), asynxModels.ErrValidation)
}

func TestSetOrder_AcceptsTheFirstSlot(t *testing.T) {
	assert.NoError(t, commands.SetOrder{ID: "n1", Order: 0}.Validate(&domain.Node{ID: "n1"}))
}

// A distinct event name from SetPlacement, so a renumber and a move are not the
// same entry in a log that is read back to explain what happened to a node.
func TestSetOrder_IsRoutedAndNamedPerAggregate(t *testing.T) {
	cmd := commands.SetOrder{ID: "n1"}

	assert.Equal(t, "n1", cmd.AggregateID())
	assert.Equal(t, "node.order_set.n1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}
