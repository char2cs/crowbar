package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestRevParse_HEADResolves(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	sha, err := e.RevParse(ctx, repo, "HEAD")

	require.NoError(t, err)
	assert.Equal(t, headSHA(t, repo), sha)
}

func TestRevParse_UnknownRef_ReturnsError(
	t *testing.T,
) {
	repo := initRepo(t)
	makeCommit(t, repo, "file.txt", "content\n", "init")
	e := git.New()
	ctx := context.Background()

	_, err := e.RevParse(ctx, repo, "refs/heads/does-not-exist")

	require.Error(t, err)
}
