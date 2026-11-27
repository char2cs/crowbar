package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestRemoteTrackingBranchExists_True(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)

	gitRun(t, repo, "branch", "feature")
	gitRun(t, repo, "push", "origin", "feature")
	gitRun(t, repo, "fetch", "origin")

	e := git.New()
	exists, err := e.RemoteTrackingBranchExists(ctx, repo, "feature")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRemoteTrackingBranchExists_FalseWhenAbsent(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)

	e := git.New()
	exists, err := e.RemoteTrackingBranchExists(ctx, repo, "does-not-exist")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRemoteTrackingBranchExists_ErrorOnBrokenRepo(
	t *testing.T,
) {
	ctx := context.Background()

	e := git.New()
	exists, err := e.RemoteTrackingBranchExists(ctx, t.TempDir(), "feature")

	require.Error(t, err)
	assert.False(t, exists)
}

func TestLocalBranchExists(t *testing.T) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)
	gitRun(t, repo, "branch", "feature")
	e := git.New()

	exists, err := e.LocalBranchExists(ctx, repo, "feature")
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = e.LocalBranchExists(ctx, repo, "develop")
	require.NoError(t, err)
	assert.False(t, exists, "a missing branch is a clean absent")
}
