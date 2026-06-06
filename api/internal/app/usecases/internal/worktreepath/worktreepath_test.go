package worktreepath

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFor_DeterministicAndSanitized(t *testing.T) {
	a := For("/home/u/proj/repo", "feature/x")
	b := For("/home/u/proj/repo", "feature/x")
	assert.Equal(t, a, b)
	assert.NotContains(t, filepath.Base(a), "/")
	assert.NotEqual(t, a, For("/home/u/proj/repo", "feature/y"))
}

func TestFor_SlashInBranchBecomesHyphen(t *testing.T) {
	got := For("/repo", "feature/foo/bar")
	base := filepath.Base(got)
	assert.False(t, strings.Contains(base, "/"), "base must not contain /")
	assert.Contains(t, base, "-")
}

func TestFor_UnsafeCharsReplaced(t *testing.T) {
	got := For("/repo", "feat:ure@branch#1")
	base := filepath.Base(got)
	assert.NotContains(t, base, ":")
	assert.NotContains(t, base, "@")
	assert.NotContains(t, base, "#")
}

func TestFor_DifferentReposDiverge(t *testing.T) {
	a := For("/home/u/proj/repoA", "main")
	b := For("/home/u/proj/repoB", "main")
	assert.NotEqual(t, a, b)
}

func TestFor_DifferentBranchesDiverge(t *testing.T) {
	a := For("/repo", "feature/x")
	b := For("/repo", "feature/y")
	assert.NotEqual(t, a, b)
}

func TestFor_RootedUnderCrowbarWorktreesSiblingDir(t *testing.T) {
	got := For("/home/u/proj/repo", "main")
	// parent of got must be .crowbar-worktrees/<repoBase>
	parent := filepath.Dir(got)
	repoBase := filepath.Base(parent)
	assert.Equal(t, "repo", repoBase)
	crowbarDir := filepath.Base(filepath.Dir(parent))
	assert.Equal(t, ".crowbar-worktrees", crowbarDir)
}
