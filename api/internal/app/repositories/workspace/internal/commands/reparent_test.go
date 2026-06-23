package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestReparent_SetsParentAndForkPoint(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", ParentID: "old", ForkPointSha: "oldfp"}
	got := commands.Reparent{ID: "ws1", ParentID: "new", ForkPointSha: "newfp"}.EmitEvent(ws)
	assert.Equal(t, "new", got.ParentID)
	assert.Equal(t, "newfp", got.ForkPointSha)
}

func TestReparent_ClearsStalePRConflicts_NoPR(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", Status: domain.WorkspaceStatusPRConflicts}
	got := commands.Reparent{ID: "ws1", ParentID: "p", ForkPointSha: "fp"}.EmitEvent(ws)
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status,
		"a reparent clears a stale conflict to new when there is no PR")
}

func TestReparent_ClearsStalePRConflicts_PreservesPR(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", Status: domain.WorkspaceStatusPRConflicts, PRUrl: "https://x/pr/1"}
	got := commands.Reparent{ID: "ws1", ParentID: "p", ForkPointSha: "fp"}.EmitEvent(ws)
	assert.Equal(t, domain.WorkspaceStatusPROpen, got.Status,
		"with a PR, clearing the conflict keeps a PR badge")
}

func TestReparent_LeavesNonConflictStatusAlone(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", Status: domain.WorkspaceStatusPROpen}
	got := commands.Reparent{ID: "ws1", ParentID: "p", ForkPointSha: "fp"}.EmitEvent(ws)
	assert.Equal(t, domain.WorkspaceStatusPROpen, got.Status,
		"reparent does not touch a non-conflict status")
}
