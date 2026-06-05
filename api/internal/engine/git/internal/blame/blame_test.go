package blame_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/blame"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()

	r, err := exec.Git(ctx, dir, "init", "-b", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode)

	_, _ = exec.Git(ctx, dir, "config", "user.email", "test@test.com")
	_, _ = exec.Git(ctx, dir, "config", "user.name", "Test User")

	return dir
}

func commitFile(
	t *testing.T,
	dir string,
	filename string,
	content string,
	message string,
) {
	t.Helper()
	ctx := context.Background()

	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	_, err := exec.Git(ctx, dir, "add", filename)
	require.NoError(t, err)

	r, err := exec.Git(ctx, dir, "commit", "-m", message)
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
}

func TestFile_SingleCommit(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "hello.txt", "line one\nline two\nline three\n", "initial commit")

	entries, err := blame.File(context.Background(), dir, "hello.txt")
	require.NoError(t, err)
	require.Len(t, entries, 3)

	for i, e := range entries {
		assert.Equal(t, i+1, e.LineNumber)
		assert.Equal(t, "Test User", e.Author)
		assert.Equal(t, "test@test.com", e.Email)
		assert.Equal(t, "initial commit", e.CommitMessage)
		assert.Len(t, e.CommitHash, 40)
		assert.False(t, e.Date.IsZero())
	}
}

func TestFile_LineCountMatchesContent(t *testing.T) {
	dir := initRepo(t)
	content := "alpha\nbeta\ngamma\ndelta\nepsilon\n"
	commitFile(t, dir, "words.txt", content, "add words")

	entries, err := blame.File(context.Background(), dir, "words.txt")
	require.NoError(t, err)

	expectedLines := 5
	assert.Len(t, entries, expectedLines)

	for i, e := range entries {
		assert.Equal(t, i+1, e.LineNumber, "line numbers must be sequential")
	}
}

func TestFile_MultipleCommits_SeparateAuthorship(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()

	commitFile(t, dir, "file.txt", "first line\n", "first commit")

	path := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("first line\nsecond line\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "file.txt")
	r, err := exec.Git(ctx, dir, "commit", "-m", "second commit")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	entries, err := blame.File(context.Background(), dir, "file.txt")
	require.NoError(t, err)
	require.Len(t, entries, 2)

	assert.Equal(t, 1, entries[0].LineNumber)
	assert.Equal(t, "first commit", entries[0].CommitMessage)

	assert.Equal(t, 2, entries[1].LineNumber)
	assert.Equal(t, "second commit", entries[1].CommitMessage)
}

func TestFile_NotExist_ReturnsError(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "real.txt", "content\n", "add file")

	_, err := blame.File(context.Background(), dir, "nonexistent.txt")
	require.Error(t, err)
}

func TestFile_SingleLine(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "one.txt", "only line\n", "single line commit")

	entries, err := blame.File(context.Background(), dir, "one.txt")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	assert.Equal(t, 1, entries[0].LineNumber)
	assert.Equal(t, "single line commit", entries[0].CommitMessage)
	assert.Equal(t, "Test User", entries[0].Author)
	assert.Equal(t, "test@test.com", entries[0].Email)
}

func TestFile_AllHashesAreSHA(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "data.txt", "a\nb\nc\n", "add data")

	entries, err := blame.File(context.Background(), dir, "data.txt")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, e := range entries {
		assert.Len(t, e.CommitHash, 40, "each entry must carry a full SHA-1")
	}
}

func TestFile_InjectedExecError(
	t *testing.T,
) {
	blame.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	_, err := blame.File(ctx, t.TempDir(), "file.txt")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "blame: file")
}

func TestIsCommitHeader_ShortSHA(
	t *testing.T,
) {
	assert.False(t, blame.ExportedIsCommitHeader("abc123 1 1"))
}

func TestIsCommitHeader_TooFewFields(
	t *testing.T,
) {
	assert.False(t, blame.ExportedIsCommitHeader("onefield"))
}

func TestIsCommitHeader_UppercaseSHA(
	t *testing.T,
) {
	sha := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	assert.False(t, blame.ExportedIsCommitHeader(sha+" 1 1"))
}

func TestParseUnixTime_InvalidInput(
	t *testing.T,
) {
	result := blame.ExportedParseUnixTime("not-a-number")
	assert.True(t, result.IsZero())
}

func TestParseUnixTime_ValidInput(
	t *testing.T,
) {
	result := blame.ExportedParseUnixTime("0")
	assert.False(t, result.IsZero())
}
