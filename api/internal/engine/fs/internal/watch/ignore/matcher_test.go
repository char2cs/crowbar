package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch/ignore"
)

// writeGitignore writes a .gitignore (or other named ignore file) with the
// given body under dir, creating parents as needed.
func writeIgnoreFile(
	t *testing.T,
	path string,
	body string,
) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func TestMatch_NoGitignore_NothingIgnored(t *testing.T) {
	repo := t.TempDir()
	m := ignore.NewMatcher(repo)

	assert.False(t, m.Match(filepath.Join(repo, "node_modules")))
	assert.False(t, m.Match(filepath.Join(repo, "src")))
}

func TestMatch_RootPattern_DirectorySlash(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "node_modules/\ndist/\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "node_modules")))
	assert.True(t, m.Match(filepath.Join(repo, "dist")))
	assert.False(t, m.Match(filepath.Join(repo, "src")))
}

func TestMatch_UnanchoredPattern_MatchesAtAnyDepth(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "build\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "build")))
	assert.True(t, m.Match(filepath.Join(repo, "src", "build")))
	assert.False(t, m.Match(filepath.Join(repo, "src")))
}

func TestMatch_AnchoredPattern_MatchesOnlyAtRoot(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "/build\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "build")))
	assert.False(t, m.Match(filepath.Join(repo, "src", "build")))
}

func TestMatch_Negation_ReincludesLaterPattern(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "build\n!build\n")
	m := ignore.NewMatcher(repo)

	assert.False(t, m.Match(filepath.Join(repo, "build")))
}

func TestMatch_Negation_OrderMatters(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "!build\nbuild\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "build")))
}

func TestMatch_NestedGitignore_AppliesBelowItsDirectory(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, "src", ".gitignore"), "generated/\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "src", "generated")))
	assert.False(t, m.Match(filepath.Join(repo, "generated")))
}

func TestMatch_GitInfoExclude_IsHonoured(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".git", "info", "exclude"), "coverage/\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "coverage")))
}

func TestMatch_CommentsAndBlankLinesIgnored(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "# a comment\n\n   \nnode_modules/\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "node_modules")))
	assert.False(t, m.Match(filepath.Join(repo, "a comment")))
}

func TestMatch_PathOutsideRepo_NotIgnored(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "build\n")
	m := ignore.NewMatcher(repo)

	assert.False(t, m.Match(repo))
	assert.False(t, m.Match(filepath.Join(repo, "..", "build")))
}

func TestMatch_DoubleStarPattern(t *testing.T) {
	repo := t.TempDir()
	writeIgnoreFile(t, filepath.Join(repo, ".gitignore"), "a/**/gen\n")
	m := ignore.NewMatcher(repo)

	assert.True(t, m.Match(filepath.Join(repo, "a", "b", "c", "gen")))
	assert.False(t, m.Match(filepath.Join(repo, "b", "gen")))
}
