package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestRemoteTrackingBranchExists_True proves the local remote-tracking ref
// refs/remotes/origin/<branch> is detected — the same universe `git branch -r`
// reads — after the branch is pushed and fetched, regardless of live reachability.
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

// TestRemoteTrackingBranchExists_FalseWhenAbsent proves a missing ref is a clean
// (false, nil), not an error — exit 1 from `show-ref --verify --quiet`.
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

// TestRemoteTrackingBranchExists_ErrorOnBrokenRepo proves a failure that is
// neither "present" nor a clean "absent" (a non-repo dir, exit 128) surfaces as
// an error rather than being silently reported as absent.
func TestRemoteTrackingBranchExists_ErrorOnBrokenRepo(
	t *testing.T,
) {
	ctx := context.Background()

	e := git.New()
	exists, err := e.RemoteTrackingBranchExists(ctx, t.TempDir(), "feature")

	require.Error(t, err)
	assert.False(t, exists)
}
