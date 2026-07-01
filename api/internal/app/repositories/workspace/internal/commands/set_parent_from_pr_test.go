package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSetParentFromPR_SetsParentID(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", ParentID: ""}
	cmd := commands.SetParentFromPR{ID: "ws1", ParentID: "parent-ws"}
	got := cmd.EmitEvent(ws)
	assert.Equal(t, "parent-ws", got.ParentID)
	assert.Equal(t, "ws1", got.ID) // other fields unchanged
}

func TestSetParentFromPR_Validate_NilWorkspace(t *testing.T) {
	cmd := commands.SetParentFromPR{ID: "ws1", ParentID: "parent-ws"}
	assert.Error(t, cmd.Validate(nil))
}

func TestSetParentFromPR_Validate_EmptyParentID(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1"}
	cmd := commands.SetParentFromPR{ID: "ws1", ParentID: ""}
	assert.Error(t, cmd.Validate(ws))
}
