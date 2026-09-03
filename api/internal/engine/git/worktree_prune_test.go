package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestWorktreePrune_RemovesStaleRegistration is WorktreePrune's only real
// behavior: a worktree directory deleted out from under git (not via `git
// worktree remove`) still has a live registration in the main repo's
// administrative files, and that stale entry must disappear from `worktree
// list` after pruning, without touching any worktree that is still on disk.
func TestWorktreePrune_RemovesStaleRegistration(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")

	wtPath := filepath.Join(t.TempDir(), "wt")
	e := git.New()
	_, err := e.WorktreeAddBranch(ctx, repo, wtPath, "feat", "HEAD")
	require.NoError(t, err)

	// Delete the worktree directory directly, bypassing `git worktree remove`,
	// so its registration under .git/worktrees is left stale.
	require.NoError(t, os.RemoveAll(wtPath))

	// git worktree list reports its own resolved (symlink-free) path, which on
	// macOS does not string-match a raw t.TempDir() path (/var vs /private/var),
	// so entries are matched by their base directory name instead.
	base := filepath.Base(wtPath)
	before, err := e.WorktreeList(ctx, repo)
	require.NoError(t, err)
	foundStale := false
	for _, entry := range before {
		if strings.HasSuffix(entry.Path, base) {
			foundStale = true
		}
	}
	require.True(t, foundStale, "the deleted worktree must still be registered before pruning")

	require.NoError(t, e.WorktreePrune(ctx, repo))

	after, err := e.WorktreeList(ctx, repo)
	require.NoError(t, err)
	for _, entry := range after {
		assert.False(t, strings.HasSuffix(entry.Path, base), "pruning must drop the registration for a worktree whose directory is gone")
	}
}
