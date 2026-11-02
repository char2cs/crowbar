package usecases_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestWorktreeChildCreator_ForcesARealWorktree_EvenFromAWorkspacelessForkParent
// pins the Promote fix: workspace.CreateChild's own taxonomy default rule
// (model spec §4.1) inherits OwnWorktree from the PARENT, so a fork parent
// that is itself a workspace-less bubble (WorktreePath == "") would otherwise
// default a promotion into ANOTHER bubble — a chat with neither a worktree nor
// a branch name, silently. worktreeChildCreator forces OwnWorktree true
// always, which this proves against the REAL workspace.Usecase (real git, real
// resolveInherited, real branch generator), not the fake usecases/chat wires
// its own fixture with.
func TestWorktreeChildCreator_ForcesARealWorktree_EvenFromAWorkspacelessForkParent(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
	require.NoError(t, err)

	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	require.NoError(t, gormStores.Repositories.Save(context.Background(), domain.Repository{
		ID:   "r1",
		Name: "repo",
		Path: repoDir,
	}))
	parent, err := repos.Workspace.Create(context.Background(), workspace.CreateInput{
		ID:        "parent-ws",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "main",
		// WorktreePath left empty on purpose: this is the workspace-less
		// "bubble" fork parent the reviewer's finding is about.
		Provisioning: domain.WorkspacePlaceholder,
	}, time.Now())
	require.NoError(t, err)
	require.Empty(t, parent.WorktreePath, "precondition: the fork parent owns no worktree of its own")

	creator := usecases.NewWorktreeChildCreatorForTest(c.Workspace)

	child, err := creator.CreateChildWorkspace(context.Background(), parent.ID, "")

	require.NoError(t, err)
	assert.NotEmpty(t, child.WorktreePath,
		"promotion must always produce a real worktree, even forked from a workspace-less parent")
	assert.NotEmpty(t, child.Branch,
		"promotion must always produce a generated branch name")
}
