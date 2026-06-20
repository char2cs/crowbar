package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// A worktree whose directory is removed out from under git leaves a stale
// (prunable) registration that keeps its branch "checked out", which otherwise
// blocks ever re-importing that branch ("already used by worktree"). WorktreeAdd
// must prune the dead registration and retry so the branch stays importable.
func TestWorktreeAdd_PrunesStaleRegistrationAndRetries(t *testing.T) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	// Register a worktree on "feat", then delete its directory so the
	// registration goes stale — exactly what happens when a workspace folder is
	// removed externally.
	stale := filepath.Join(t.TempDir(), "stale")
	_, err := e.WorktreeAddBranch(ctx, repo, stale, "feat", "HEAD")
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(stale))

	// Re-adding "feat" at a fresh path must succeed (prune + retry), not fail
	// with "already used by worktree".
	fresh := filepath.Join(t.TempDir(), "fresh")
	require.NoError(t, e.WorktreeAdd(ctx, repo, fresh, "feat"))

	entries, err := e.WorktreeList(ctx, repo)
	require.NoError(t, err)
	live := 0
	for _, en := range entries {
		if en.Branch == "feat" && !en.Prunable {
			live++
		}
	}
	require.Equal(t, 1, live, "exactly one live worktree should remain on feat")
}
