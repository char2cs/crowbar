package diff_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

func TestRange_HappyPath(t *testing.T) {
	dir := initRepo(t)

	// base branch: one commit
	writeFile(t, dir, "base.go", "package main\n")
	mustGit(t, dir, "add", "base.go")
	mustGit(t, dir, "commit", "-m", "base commit")

	// feature branch: add a new file
	mustGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "feature.go", "package main\n\nfunc feature() {}\n")
	mustGit(t, dir, "add", "feature.go")
	mustGit(t, dir, "commit", "-m", "add feature file")

	ctx := context.Background()
	result, err := diff.Range(ctx, dir, "main", "feature")
	require.NoError(t, err)

	require.Len(t, result.Files, 1)
	assert.Equal(t, "feature.go", result.Files[0].FilePath)
	assert.True(t, result.Files[0].IsNew)
	assert.Greater(t, result.Files[0].Additions, 0)
	assert.Equal(t, 1, result.TotalFiles)
	assert.Greater(t, result.TotalAdditions, 0)
	assert.Equal(t, 0, result.TotalDeletions)

	// commit metadata fields must be zero-value (Range has no commit)
	assert.Empty(t, result.CommitHash)
	assert.Empty(t, result.CommitMessage)
}

func TestRange_ErrorBadBase(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "file.go", "package main\n")
	mustGit(t, dir, "add", "file.go")
	mustGit(t, dir, "commit", "-m", "init")

	ctx := context.Background()
	_, err := diff.Range(ctx, dir, "nonexistent-base", "main")
	require.Error(t, err)
}
