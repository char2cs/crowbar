package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestResolveConflicts_ClearsPRConflicts_NoPR(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", Status: domain.WorkspaceStatusPRConflicts}
	got := commands.ResolveConflicts{ID: "ws1"}.EmitEvent(ws)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status,
		"resolving the local conflict clears pr-conflicts to new when there is no PR")
}

func TestResolveConflicts_ClearsPRConflicts_PreservesPR(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", Status: domain.WorkspaceStatusPRConflicts, PRUrl: "https://x/pr/1"}
	got := commands.ResolveConflicts{ID: "ws1"}.EmitEvent(ws)
	assert.Equal(t, domain.WorkspaceStatusPROpen, got.Status,
		"with a PR, clearing the conflict keeps the PR badge")
}

func TestResolveConflicts_LeavesNonConflictStatusAlone(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", Status: domain.WorkspaceStatusPROpen}
	got := commands.ResolveConflicts{ID: "ws1"}.EmitEvent(ws)
	assert.Equal(t, domain.WorkspaceStatusPROpen, got.Status,
		"resolve does not touch a non-conflict status")
}

func TestResolveConflicts_ValidateNilCurrent(t *testing.T) {
	require.Error(t, commands.ResolveConflicts{ID: "ws1"}.Validate(nil),
		"a resolve on an unknown aggregate must fail validation")
}
