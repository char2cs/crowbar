package git_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// initRepoWithBareOrigin creates a working repo with one commit on main and a
// bare remote wired as `origin`, returning the working repo path.
func initRepoWithBareOrigin(
	t *testing.T,
) string {
	t.Helper()
	repo := initRepo(t)
	makeCommit(t, repo, "base.txt", "base\n", "base")

	bare := filepath.Join(t.TempDir(), "origin.git")
	gitRun(t, t.TempDir(), "init", "--bare", "-b", "main", bare)
	gitRun(t, repo, "remote", "add", "origin", bare)
	gitRun(t, repo, "push", "origin", "main")
	return repo
}

func TestRemoteBranchExists_True(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)

	gitRun(t, repo, "branch", "feature")
	gitRun(t, repo, "push", "origin", "feature")

	e := git.New()
	exists, err := e.RemoteBranchExists(ctx, repo, "feature")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRemoteBranchExists_False(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)

	e := git.New()
	exists, err := e.RemoteBranchExists(ctx, repo, "does-not-exist")

	require.NoError(t, err)
	assert.False(t, exists)
}
