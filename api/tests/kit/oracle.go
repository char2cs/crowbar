//go:build integration

package kit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// projectDirOf mirrors worktreepath.ProjectDir for the oracle's path-shape
// assertion. The worktreepath package is doubly-internal (under
// app/usecases/internal) and therefore not importable from tests/kit, so the
// UUID layout is reconstructed here from the same contract (spec §1/§8).
func projectDirOf(
	home string,
	projectID string,
) string {
	return filepath.Join(home, "projects", projectID)
}

// worktreePathOf mirrors worktreepath.For: the crowbar-created worktree dir for
// a workspace, <home>/projects/<P>/<R>/workspaces/<W>/worktree.
func worktreePathOf(
	home string,
	projectID string,
	repoID string,
	wsID string,
) string {
	return filepath.Join(
		projectDirOf(home, projectID),
		repoID,
		"workspaces",
		wsID,
		"worktree",
	)
}

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
		// A crowbar-created child worktree (one living under the crowbar home's
		// projects tree) must follow the UUID layout (spec §1/§8):
		// <home>/projects/<P>/<R>/workspaces/<W>/worktree. An adopted main
		// worktree points at the user's real on-disk repo path and is exempt.
		if strings.HasPrefix(ws.WorktreePath, projectDirOf(env.homeDir, ws.ProjectID)) {
			assert.Equal(
				t,
				worktreePathOf(
					env.homeDir,
					ws.ProjectID,
					ws.RepoID,
					ws.ID,
				),
				ws.WorktreePath,
				"oracle: crowbar-created worktree path must equal the UUID layout",
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

// AssertGitStateMatchesReadModel verifies that a conflicted git worktree is
// reflected by the workspace read-model's status (spec §5): if the worktree has
// conflicted files, the workspace Status must be pr-conflicts. (HasConflicts was
// removed; conflict state is now carried by the status enum.)
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

	if hasConflicts {
		assert.Equal(
			t,
			domain.WorkspaceStatusPRConflicts,
			ws.Status,
			"oracle: conflicted worktree must set status pr-conflicts for workspace %s",
			wsID,
		)
	}
}

