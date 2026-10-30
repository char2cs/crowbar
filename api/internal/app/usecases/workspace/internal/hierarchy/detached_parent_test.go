package hierarchy_test

import (
	"context"
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
