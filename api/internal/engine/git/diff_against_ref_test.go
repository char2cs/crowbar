package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestDiffAgainstRef_IncludesCommittedAndUncommitted(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "base.go", "package main\n", "base commit")

	gitRun(t, dir, "checkout", "-b", "feature")
	makeCommit(t, dir, "committed.go", "package main\n\nfunc c() {}\n", "committed change")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.go"), []byte("package main\n// edit\n"), 0o644))

	ctx := context.Background()
	e := git.New()
	result, err := e.DiffAgainstRef(ctx, dir, "main")
	require.NoError(t, err)

	paths := map[string]bool{}
	for _, f := range result.Files {
		paths[f.FilePath] = true
	}
	assert.True(t, paths["committed.go"], "committed branch change must appear")
	assert.True(t, paths["base.go"], "uncommitted working-tree edit must appear")
	assert.GreaterOrEqual(t, result.TotalFiles, 2)
}
