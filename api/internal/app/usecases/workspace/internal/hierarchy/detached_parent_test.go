package hierarchy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A branchless parent that is NOT the repo folder forks from its own commit,
// not from whatever the repo folder happens to hold.
func TestCreateChild_OffABranchlessManagedParent_ForksFromThatParentsCommit(t *testing.T) {
	repoPath, _ := setupImportRepo(t)
	parentPath := filepath.Join(t.TempDir(), "detached")
	gitRun(t, repoPath, "worktree", "add", "--detach", parentPath, "origin/feature/x")
	parentHead := revParse(t, parentPath, "HEAD")
	require.NotEqual(t, revParse(t, repoPath, "HEAD"), parentHead)
	uc, workspaces, _ := homeBranchHarness(t, repoPath)
	ctx := context.Background()

	_, err := workspaces.Create(ctx, workspace.CreateInput{
		ID:           "detached",
		RepoID:       "r1",
		ProjectID:    "p1",
		WorktreePath: parentPath,
		Provisioning: domain.WorkspaceProvisioned,
	}, time.Unix(1000, 0).UTC())
	require.NoError(t, err)

	ownWorktree := true
	child, err := uc.CreateChild(ctx, hierarchy.CreateChildInput{
		ParentID:    "detached",
		Branch:      "off-detached",
		OwnWorktree: &ownWorktree,
	})
	require.NoError(t, err)
	assert.Equal(t, parentHead, child.ForkPointSha)
}

// A thread made a branch starts from what its folder has committed: the parent's
// local HEAD, never origin's tip, with its uncommitted edits left where they are.
func TestCreateChild_FromParentHead_ForksFromTheLocalCommitNotOrigin(t *testing.T) {
	repoPath, _ := setupImportRepo(t)
	parentPath := filepath.Join(t.TempDir(), "feature")
	gitRun(t, repoPath, "worktree", "add", "-b", "feature/x", parentPath, "origin/feature/x")
	gitRun(t, parentPath, "config", "user.email", "t@t")
	gitRun(t, parentPath, "config", "user.name", "t")
	writeFile(t, filepath.Join(parentPath, "f.txt"), "LOCAL-AHEAD")
	gitRun(t, parentPath, "commit", "-am", "local only")
	writeFile(t, filepath.Join(parentPath, "f.txt"), "UNCOMMITTED")
	localHead := revParse(t, parentPath, "HEAD")
	require.NotEqual(t, revParse(t, repoPath, "origin/feature/x"), localHead)
	uc, workspaces, _ := homeBranchHarness(t, repoPath)
	ctx := context.Background()
	_, err := workspaces.Create(ctx, workspace.CreateInput{
		ID: "feature", RepoID: "r1", ProjectID: "p1", Branch: "feature/x",
		WorktreePath: parentPath, Provisioning: domain.WorkspaceProvisioned,
	}, time.Unix(1000, 0).UTC())
	require.NoError(t, err)

	ownWorktree := true
	child, err := uc.CreateChild(ctx, hierarchy.CreateChildInput{
		ParentID: "feature", Branch: "made-a-branch", OwnWorktree: &ownWorktree, FromParentHead: true,
	})
	require.NoError(t, err)
	assert.Equal(t, localHead, child.ForkPointSha)
	body, err := os.ReadFile(filepath.Join(child.WorktreePath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "LOCAL-AHEAD", string(body), "uncommitted edits are not copied")
	body, err = os.ReadFile(filepath.Join(parentPath, "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "UNCOMMITTED", string(body), "the parent is untouched")
}
