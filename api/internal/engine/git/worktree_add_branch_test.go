package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestWorktreeAddBranch_CreatesBranchAndReturnsStartSha(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	child := filepath.Join(t.TempDir(), "child")
	startSha, err := e.WorktreeAddBranch(ctx, repo, child, "feature/x", "HEAD")

	require.NoError(t, err)
	assert.NotEmpty(t, startSha)
	assert.Equal(t, headSHA(t, repo), startSha)

	entries, err := e.WorktreeList(ctx, repo)
	require.NoError(t, err)
	found := false
	for _, en := range entries {
		if en.Branch == "feature/x" {
			found = true
		}
	}
	assert.True(t, found, "worktree on branch feature/x not found")
}

func TestWorktreeAddBranch_InvalidStartPoint_ReturnsError(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	child := filepath.Join(t.TempDir(), "child")
	_, err := e.WorktreeAddBranch(ctx, repo, child, "feature/y", "nonexistent-ref")

	require.Error(t, err)
}

// TestWorktreeAdd_ParentsItCreatesAreOwnerOnly: the directories a worktree add
// creates above the checkout are Crowbar's scaffolding, so they are 0700; the
// checkout itself is git's and keeps the umask.
func TestWorktreeAdd_ParentsItCreatesAreOwnerOnly(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()
	root := t.TempDir()

	_, err := e.WorktreeAddBranch(ctx, repo, filepath.Join(root, "p", "a", "worktree"), "feature/a", "HEAD")
	require.NoError(t, err)
	_, err = e.WorktreeAddAtRef(ctx, repo, filepath.Join(root, "p", "b", "worktree"), "feature/b", "HEAD")
	require.NoError(t, err)
	gitRun(t, repo, "branch", "feature/c")
	require.NoError(t, e.WorktreeAdd(ctx, repo, filepath.Join(root, "p", "c", "worktree"), "feature/c"))

	for _, dir := range []string{"p", "p/a", "p/b", "p/c"} {
		info, err := os.Stat(filepath.Join(root, dir))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), dir)
	}
}

// In a Crowbar home the directories down to a checkout are traversable by other
// users, never listable: an IDE or a container running as another uid reaches
// the checkout as git made it, and nothing above the home changes.
func TestWorktreeAdd_ACheckoutInACrowbarHomeStaysReachable(
	t *testing.T,
) {
	testutil.RequirePermissionEnforcement(t)
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	above := t.TempDir()
	home := filepath.Join(above, "crowbar")
	require.NoError(t, os.MkdirAll(filepath.Join(home, "projects"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "LAYOUT"), []byte("1\n"), 0o600))
	require.NoError(t, os.Chmod(home, 0o700))
	require.NoError(t, os.Chmod(above, 0o700))

	_, err := e.WorktreeAddBranch(context.Background(), repo,
		filepath.Join(home, "projects", "p1", "acme", "main", "worktree"), "feature/a", "HEAD")
	require.NoError(t, err)

	for _, dir := range []string{"", "projects", "projects/p1", "projects/p1/acme", "projects/p1/acme/main"} {
		info, err := os.Stat(filepath.Join(home, dir))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o711), info.Mode().Perm(), dir)
	}
	info, err := os.Stat(above)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "nothing above the home")
}
