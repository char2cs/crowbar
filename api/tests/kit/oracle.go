//go:build integration

package kit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectDirOf mirrors worktreepath.ProjectDir for the oracle's path-shape assertion,
// rebuilt from the layout contract rather than imported.
func projectDirOf(
	home string,
	projectID string,
) string {
	return filepath.Join(home, "projects", projectID)
}

// worktreePathOf mirrors worktreepath.Derive for a no-remote repo:
// <home>/projects/<project>/<base(repoPath)>/<branch>/worktree. It is rebuilt from the
// layout contract rather than imported, so the oracle never echoes the code under test.
func worktreePathOf(
	home string,
	projectID string,
	repoPath string,
	branch string,
) string {
	return filepath.Join(
		projectDirOf(home, projectID),
		filepath.Base(repoPath),
		branch,
		"worktree",
	)
}

// AssertWorkspaceConsistency verifies that wsID's read model matches disk: it is in
// Get and List, its worktree dir exists, and the on-disk branch matches Branch.
func AssertWorkspaceConsistency(
	t *testing.T,
	env *Env,
	repoPath string,
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
		// A Crowbar-created child worktree must follow <home>/projects/<project>/<slug>/<branch>;
		// an adopted main worktree points at the user's own repo path and is exempt.
		if strings.HasPrefix(ws.WorktreePath, projectDirOf(env.homeDir, ws.ProjectID)) {
			assert.Equal(
				t,
				worktreePathOf(
					env.homeDir,
					ws.ProjectID,
					repoPath,
					ws.Branch,
				),
				ws.WorktreePath,
				"oracle: crowbar-created worktree path must equal the friendly host/owner/repo/branch layout",
			)
		}

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
