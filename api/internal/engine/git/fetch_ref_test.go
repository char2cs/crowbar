package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestFetchRef_MakesRemoteBranchAvailable proves FetchRef pulls a single branch
// from origin so that origin/<branch> resolves locally afterwards.
func TestFetchRef_MakesRemoteBranchAvailable(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)

	// Push a feature branch to origin from a SEPARATE clone so the primary repo
	// has no local ref for it until FetchRef runs.
	bare := gitOutput(t, repo, "remote", "get-url", "origin")
	other := initRepo(t)
	gitRun(t, other, "remote", "add", "origin", bare)
	gitRun(t, other, "fetch", "origin")
	gitRun(t, other, "checkout", "-b", "feature", "origin/main")
	makeCommit(t, other, "f.txt", "feature\n", "feature work")
	gitRun(t, other, "push", "origin", "feature")

	e := git.New()
	require.NoError(t, e.FetchRef(ctx, repo, "feature"))

	sha, err := e.RevParse(ctx, repo, "origin/feature")
	require.NoError(t, err)
	assert.NotEmpty(t, sha)
}

func TestFetchRef_UnknownBranchErrors(
	t *testing.T,
) {
	ctx := context.Background()
	repo := initRepoWithBareOrigin(t)

	e := git.New()
	err := e.FetchRef(ctx, repo, "no-such-branch")
	require.Error(t, err)
}
