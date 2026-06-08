//go:build integration

package kit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// AssertWorkspaceConsistency verifies that the workspace read-model is consistent
// with what is stored on disk for wsID.
//
// It checks:
//   - The workspace exists in the read-model (Get)
//   - The workspace appears in List
//   - The worktree directory exists on disk
//   - The current branch on disk matches Branch in the read model
func AssertWorkspaceConsistency(
	t *testing.T,
	env *Env,
	wsID string,
) {
	t.Helper()
	ctx := context.Background()

	ws, err := env.app.Repositories.Workspace.Get(
		ctx,
		wsID,
	)
	require.NoError(
		t,
		err,
		"oracle: get workspace",
	)

	list, err := env.app.Repositories.Workspace.List(
		ctx,
	)
	require.NoError(
		t,
		err,
		"oracle: list workspaces",
	)

	found := false
	for _, row := range list {
		if row.ID == wsID {
			found = true
			break
		}
	}
	assert.True(
		t,
		found,
		"oracle: workspace %s not found in List",
		wsID,
	)

	if ws.WorktreePath != "" {
		dirOK := DirExists(
			t,
			ws.WorktreePath,
		)
		assert.True(
			t,
			dirOK,
			"oracle: worktree path %s does not exist on disk",
			ws.WorktreePath,
		)

		diskBranch := BranchName(
			t,
			ws.WorktreePath,
		)
		assert.Equal(
			t,
			ws.Branch,
			diskBranch,
			"oracle: branch in read-model (%s) disagrees with disk (%s)",
			ws.Branch,
			diskBranch,
		)
	}
}

// AssertGitStateMatchesReadModel verifies that the git status of wsID's worktree
// matches the HasConflicts flag stored in the workspace read-model.
func AssertGitStateMatchesReadModel(
	t *testing.T,
	env *Env,
	wsID string,
) {
	t.Helper()
	ctx := context.Background()

	ws, err := env.app.Repositories.Workspace.Get(
		ctx,
		wsID,
	)
	require.NoError(
		t,
		err,
		"oracle: get workspace for git state check",
	)

	if ws.WorktreePath == "" {
		return
	}

	status, err := env.engine.Git.Status(
		ctx,
		ws.WorktreePath,
	)
	require.NoError(
		t,
		err,
		"oracle: git status",
	)

	hasConflicts := false
	for _, f := range status.Files {
		if f.Status == gitdomain.GitFileStatusConflicted {
			hasConflicts = true
			break
		}
	}

	assert.Equal(
		t,
		ws.HasConflicts,
		hasConflicts,
		"oracle: HasConflicts mismatch for workspace %s",
		wsID,
	)
}

