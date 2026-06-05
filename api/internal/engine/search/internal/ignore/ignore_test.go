package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/ignore"
)

func writeGitignore(
	t *testing.T,
	dir string,
	content string,
) string {
	t.Helper()
	path := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestStack_AlwaysIgnoresDotGit(t *testing.T) {
	var s ignore.Stack
	assert.True(t, s.ShouldIgnore("/repo/.git/config"))
	assert.True(t, s.ShouldIgnore("/repo/.git"))
}

func TestStack_EmptyStack_NothingIgnored(t *testing.T) {
	var s ignore.Stack
	assert.False(t, s.ShouldIgnore("/repo/src/main.go"))
}

func TestStack_Add_MissingFileIsOK(t *testing.T) {
	var s ignore.Stack
	err := s.Add("/repo", "/repo/.gitignore") // doesn't exist
	require.NoError(t, err)
}

func TestStack_SimplePattern(t *testing.T) {
	dir := t.TempDir()
	path := writeGitignore(t, dir, "vendor/\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "vendor", "lib.go")))
	assert.False(t, s.ShouldIgnore(filepath.Join(dir, "src", "main.go")))
}

func TestStack_NegatedPattern(t *testing.T) {
	dir := t.TempDir()
	path := writeGitignore(t, dir, "*.log\n!important.log\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "debug.log")))
	assert.False(t, s.ShouldIgnore(filepath.Join(dir, "important.log")))
}

func TestStack_WildcardPattern(t *testing.T) {
	dir := t.TempDir()
	path := writeGitignore(t, dir, "*.bin\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "binary.bin")))
	assert.False(t, s.ShouldIgnore(filepath.Join(dir, "main.go")))
}

func TestStack_CommentAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := writeGitignore(t, dir, "# this is a comment\n\n*.log\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "app.log")))
	assert.False(t, s.ShouldIgnore(filepath.Join(dir, "app.go")))
}

func TestStack_AnchoredPattern(t *testing.T) {
	dir := t.TempDir()
	// Anchored: only matches "build/" at the root of the repo.
	path := writeGitignore(t, dir, "/build/\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "build", "output.o")))
}

func TestStack_ChildOutsideLayerDir(t *testing.T) {
	dir := t.TempDir()
	path := writeGitignore(t, dir, "*.log\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	// Path outside the layer directory should not be ignored.
	assert.False(t, s.ShouldIgnore("/completely/different/path.log"))
}

func TestStack_Add_ScanError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(path, []byte("vendor/\n"), 0o600))

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))
}

func TestStack_EscapedExclamation(t *testing.T) {
	dir := t.TempDir()
	// A line starting with \! should be treated as a literal "!" character.
	path := writeGitignore(t, dir, `\!important.log`+"\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "!important.log")))
}

func TestStack_AnchoredPatternMatchesFile(t *testing.T) {
	dir := t.TempDir()
	// /build matches build/output.o
	path := writeGitignore(t, dir, "/build\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "build")))
	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "build", "output.o")))
	assert.False(t, s.ShouldIgnore(filepath.Join(dir, "src", "build.go")))
}

func TestStack_Add_PermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, cannot test permission denial")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(path, []byte("vendor/\n"), 0o000))
	defer func() { _ = os.Chmod(path, 0o600) }()

	var s ignore.Stack
	err := s.Add(dir, path)
	require.Error(t, err)
}

func TestStack_SuffixPatternMatchesRelative(t *testing.T) {
	dir := t.TempDir()
	// Pattern "*.go" should match nested files like "sub/main.go"
	path := writeGitignore(t, dir, "*.go\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "sub", "main.go")))
}

func TestStack_AllSpacesLineIgnored(t *testing.T) {
	dir := t.TempDir()
	// A line that is only spaces should be treated as blank (not a pattern).
	path := writeGitignore(t, dir, "   \n*.log\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	// Only *.log should be active.
	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "app.log")))
	assert.False(t, s.ShouldIgnore(filepath.Join(dir, "app.go")))
}

func TestStack_SuffixGlobMatchesDeep(t *testing.T) {
	dir := t.TempDir()
	// Non-anchored pattern "sub/*.go" contains a slash → anchored.
	// But "sub/main" matches "sub/main.go" via anchored+doublestar.
	// Test the suffix path: non-anchored pattern matching relative path.
	path := writeGitignore(t, dir, "main.go\n")

	var s ignore.Stack
	require.NoError(t, s.Add(dir, path))

	// Should match "a/b/main.go" via suffix matching (suffix = "main.go").
	assert.True(t, s.ShouldIgnore(filepath.Join(dir, "a", "b", "main.go")))
}
