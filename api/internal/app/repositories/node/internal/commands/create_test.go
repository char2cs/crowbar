package commands_test

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/node/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var _ asynxModels.Command[domain.Node] = commands.Create{}

func TestCreate_Validate_RejectsUnknownKind(t *testing.T) {
	c := commands.Create{ID: "n1", Kind: domain.NodeKind("bogus")}
	assert.ErrorIs(t, c.Validate(nil), asynxModels.ErrValidation)
}

func TestCreate_Validate_AcceptsEachKnownKind(t *testing.T) {
	for _, k := range []domain.NodeKind{
		domain.NodeKindChat,
		domain.NodeKindFolder,
		domain.NodeKindWorkspace,
		domain.NodeKindRepo,
	} {
		c := commands.Create{ID: "n1", Kind: k}
		if err := c.Validate(nil); err != nil {
			t.Fatalf("kind %s: unexpected error: %v", k, err)
		}
	}
}

func TestCreate_Validate_RejectsExisting(t *testing.T) {
	c := commands.Create{ID: "n1", Kind: domain.NodeKindChat}
	assert.ErrorIs(t, c.Validate(&domain.Node{ID: "n1"}), asynxModels.ErrValidation)
}

func TestCreate_Validate_RejectsMissingID(t *testing.T) {
	c := commands.Create{Kind: domain.NodeKindChat}
	assert.ErrorIs(t, c.Validate(nil), asynxModels.ErrValidation)
}

func TestCreate_Validate_RejectsNegativeOrder(t *testing.T) {
	c := commands.Create{ID: "n1", Kind: domain.NodeKindChat, Order: -1}
	assert.ErrorIs(t, c.Validate(nil), asynxModels.ErrValidation)
}

func TestCreate_EmitEvent_BuildsTheNode(t *testing.T) {
	c := commands.Create{ID: "n1", Kind: domain.NodeKindFolder, ParentID: "p1", Order: 2}

	out := c.EmitEvent(nil)

	require.Equal(t, "n1", out.ID)
	assert.Equal(t, domain.NodeKindFolder, out.Kind)
	assert.Equal(t, "p1", out.ParentID)
	assert.Equal(t, 2, out.Order)
}

func TestCreate_IsRoutedAndNamedPerAggregate(t *testing.T) {
	cmd := commands.Create{ID: "n1"}

	assert.Equal(t, "n1", cmd.AggregateID())
	assert.Equal(t, "node.created.n1", cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}
