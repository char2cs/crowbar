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

func TestWorktreePrune_RemovesStaleRegistration(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")

	wtPath := filepath.Join(t.TempDir(), "wt")
	e := git.New()
	_, err := e.WorktreeAddBranch(ctx, repo, wtPath, "feat", "HEAD")
	require.NoError(t, err)

	require.NoError(t, os.RemoveAll(wtPath))

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
