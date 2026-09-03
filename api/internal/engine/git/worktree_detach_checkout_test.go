package git_test

import (
	"context"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// currentBranch returns the branch checked out at dir, or "" on a detached
// HEAD (`git symbolic-ref` fails when there is no symbolic ref to resolve).
// Used instead of WorktreeList path-matching because `git worktree list`
// reports the worktree's resolved (symlink-free) path, which does not always
// string-match a raw t.TempDir() path (e.g. macOS's /var -> /private/var).
func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := osexec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out[:len(out)-1])
}

// TestDetachWorktree_ThenCheckoutBranch covers the round trip DetachWorktree /
// CheckoutBranch exist for: freeing a branch from one worktree's HEAD (so it
// can be attached to a different, newly managed worktree) and then restoring
// it.
func TestDetachWorktree_ThenCheckoutBranch(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")

	wtPath := filepath.Join(t.TempDir(), "wt")
	_, err := git.New().WorktreeAddBranch(ctx, repo, wtPath, "feat", "HEAD")
	require.NoError(t, err)
	require.Equal(t, "feat", currentBranch(t, wtPath))

	e := git.New()
	require.NoError(t, e.DetachWorktree(ctx, wtPath))
	assert.Empty(t, currentBranch(t, wtPath), "worktree must be on a detached HEAD, holding no branch")

	require.NoError(t, e.CheckoutBranch(ctx, wtPath, "feat"))
	assert.Equal(t, "feat", currentBranch(t, wtPath), "checking back out to feat must re-attach the branch")
}

// TestCheckoutBranch_AlreadyCheckedOutElsewhere_ReturnsError covers
// CheckoutBranch's error path: a branch already checked out in another
// worktree cannot also be checked out here.
func TestCheckoutBranch_AlreadyCheckedOutElsewhere_ReturnsError(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")

	first := filepath.Join(t.TempDir(), "first")
	e := git.New()
	_, err := e.WorktreeAddBranch(ctx, repo, first, "feat", "HEAD")
	require.NoError(t, err)

	second := filepath.Join(t.TempDir(), "second")
	_, err = e.WorktreeAddBranch(ctx, repo, second, "other", "HEAD")
	require.NoError(t, err)

	err = e.CheckoutBranch(ctx, second, "feat")

	assert.Error(t, err)
}

func TestIsStaleWorktreeConflict_NilError(t *testing.T) {
	assert.False(t, git.ExportedIsStaleWorktreeConflict(nil))
}
