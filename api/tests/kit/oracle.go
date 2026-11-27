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

func projectDirOf(
	home string,
	projectID string,
) string {
	return filepath.Join(home, "projects", projectID)
}

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
