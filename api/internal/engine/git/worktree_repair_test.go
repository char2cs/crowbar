package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// A workspace root holds the git worktree and the agent `chats` tree as
// siblings, so a branch rename relocates the whole root in one os.Rename and
// then repairs git's bookkeeping. This pins that the repair leaves a fully
// usable worktree at the new path — still on its branch, with chat state
// carried along.
func TestWorktreeRepair_RepointsRegistrationAfterRootRename(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	leaves := t.TempDir()
	oldRoot := filepath.Join(leaves, "testing")
	require.NoError(t, os.MkdirAll(oldRoot, 0o755))
	_, err := e.WorktreeAddBranch(ctx, repo, filepath.Join(oldRoot, "worktree"), "testing", "HEAD")
	require.NoError(t, err)
	// Agent state lives beside the worktree, inside the same root.
	require.NoError(t, os.MkdirAll(filepath.Join(oldRoot, "chats"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(oldRoot, "chats", "history.json"), []byte("agent state"), 0o600,
	))

	newRoot := filepath.Join(leaves, "feature", "x")
	require.NoError(t, os.MkdirAll(filepath.Dir(newRoot), 0o755))
	require.NoError(t, os.Rename(oldRoot, newRoot))

	newWorktree := filepath.Join(newRoot, "worktree")
	require.NoError(t, e.WorktreeRepair(ctx, repo, newWorktree))

	// git's registration follows the tree, still on the same branch.
	wantPath, err := filepath.EvalSymlinks(newWorktree)
	require.NoError(t, err)
	entries, err := e.WorktreeList(ctx, repo)
	require.NoError(t, err)
	var found bool
	for _, en := range entries {
		if en.Branch == "testing" {
			found = true
			assert.Equal(t, wantPath, en.Path, "registration must point at the relocated worktree")
		}
	}
	assert.True(t, found, "worktree on branch testing not found after repair")

	// The relocated worktree is actually usable, not just registered.
	status, err := e.Status(ctx, newWorktree)
	require.NoError(t, err, "relocated worktree must answer git status")
	assert.Equal(t, "testing", status.Branch)

	// Chat state travelled with the root rather than being orphaned.
	chats, err := os.ReadFile(filepath.Join(newRoot, "chats", "history.json"))
	require.NoError(t, err)
	assert.Equal(t, "agent state", string(chats))
}
