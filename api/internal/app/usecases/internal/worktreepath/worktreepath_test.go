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

// TestFor_InjectiveOnSanitisationCollision verifies that two branch names which
// are identical after unsafe-character replacement still map to distinct paths.
func TestFor_InjectiveOnSanitisationCollision(t *testing.T) {
	slash := For("/repo", "feature/foo")
	hyphen := For("/repo", "feature-foo")
	assert.NotEqual(t, slash, hyphen,
		"feature/foo and feature-foo must not share a worktree directory")
}

// TestFor_StableAcrossCalls verifies that the same inputs always produce the
// same path — a basic idempotency check for the hash suffix.
func TestFor_StableAcrossCalls(t *testing.T) {
	first := For("/home/u/proj/repo", "feature/foo")
	second := For("/home/u/proj/repo", "feature/foo")
	assert.Equal(t, first, second, "For must be deterministic")
}
