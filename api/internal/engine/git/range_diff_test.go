package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestRangeDiff_HappyPath(t *testing.T) {
	dir := initRepo(t)

	// base branch: one commit
	makeCommit(t, dir, "base.go", "package main\n", "base commit")

	// feature branch: add a new file
	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "feature.go", "package main\n\nfunc feature() {}\n", "add feature file")

	ctx := context.Background()
	e := git.New()
	result, err := e.RangeDiff(ctx, dir, "main", "feature")
	require.NoError(t, err)

	require.Len(t, result.Files, 1)
	assert.Equal(t, "feature.go", result.Files[0].FilePath)
	assert.True(t, result.Files[0].IsNew)
	assert.Greater(t, result.Files[0].Additions, 0)
	assert.Equal(t, 1, result.TotalFiles)
	assert.Greater(t, result.TotalAdditions, 0)
}

func TestRangeDiff_ErrorBadBase(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "file.go", "package main\n", "init")

	ctx := context.Background()
	e := git.New()
	_, err := e.RangeDiff(ctx, dir, "nonexistent-base", "main")
	require.Error(t, err)
}
