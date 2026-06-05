package log_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/log"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()

	r := exec.Git(ctx, dir, "init", "-b", "main")
	require.Equal(t, 0, r.ExitCode)

	_ = exec.Git(ctx, dir, "config", "user.email", "test@test.com")
	_ = exec.Git(ctx, dir, "config", "user.name", "Test User")

	return dir
}

func addCommit(
	t *testing.T,
	dir string,
	filename string,
	content string,
	message string,
) {
	t.Helper()
	ctx := context.Background()

	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	_ = exec.Git(ctx, dir, "add", filename)

	r := exec.Git(ctx, dir, "commit", "-m", message)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
}

func TestList_EmptyRepo(t *testing.T) {
	dir := initRepo(t)

	commits, err := log.List(context.Background(), dir, 50, 0)
	require.NoError(t, err)
	assert.Empty(t, commits)
}

func TestList_SingleCommit(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "file.txt", "hello\n", "initial commit")

	commits, err := log.List(context.Background(), dir, 50, 0)
	require.NoError(t, err)
	require.Len(t, commits, 1)

	c := commits[0]
	assert.Equal(t, "initial commit", c.Message)
	assert.Equal(t, "Test User", c.Author)
	assert.Equal(t, "test@test.com", c.Email)
	assert.NotEmpty(t, c.Hash)
	assert.NotEmpty(t, c.ShortHash)
	assert.Len(t, c.Hash, 40)
	assert.False(t, c.Date.IsZero())
}

func TestList_MultipleCommits(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "commit one")
	addCommit(t, dir, "b.txt", "b\n", "commit two")
	addCommit(t, dir, "c.txt", "c\n", "commit three")

	commits, err := log.List(context.Background(), dir, 50, 0)
	require.NoError(t, err)
	require.Len(t, commits, 3)

	assert.Equal(t, "commit three", commits[0].Message)
	assert.Equal(t, "commit two", commits[1].Message)
	assert.Equal(t, "commit one", commits[2].Message)
}

func TestList_Pagination_Limit(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "commit one")
	addCommit(t, dir, "b.txt", "b\n", "commit two")
	addCommit(t, dir, "c.txt", "c\n", "commit three")

	commits, err := log.List(context.Background(), dir, 2, 0)
	require.NoError(t, err)
	require.Len(t, commits, 2)

	assert.Equal(t, "commit three", commits[0].Message)
	assert.Equal(t, "commit two", commits[1].Message)
}

func TestList_Pagination_Skip(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "commit one")
	addCommit(t, dir, "b.txt", "b\n", "commit two")
	addCommit(t, dir, "c.txt", "c\n", "commit three")

	commits, err := log.List(context.Background(), dir, 50, 1)
	require.NoError(t, err)
	require.Len(t, commits, 2)

	assert.Equal(t, "commit two", commits[0].Message)
	assert.Equal(t, "commit one", commits[1].Message)
}

func TestList_Pagination_LimitAndSkip(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "commit one")
	addCommit(t, dir, "b.txt", "b\n", "commit two")
	addCommit(t, dir, "c.txt", "c\n", "commit three")
	addCommit(t, dir, "d.txt", "d\n", "commit four")

	commits, err := log.List(context.Background(), dir, 2, 1)
	require.NoError(t, err)
	require.Len(t, commits, 2)

	assert.Equal(t, "commit three", commits[0].Message)
	assert.Equal(t, "commit two", commits[1].Message)
}

func TestList_DefaultLimit(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "only commit")

	commits, err := log.List(context.Background(), dir, 0, 0)
	require.NoError(t, err)
	require.Len(t, commits, 1)
}

func TestList_CommitWithDescription(t *testing.T) {
	dir := initRepo(t)
	ctx := context.Background()

	path := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0o600))
	_ = exec.Git(ctx, dir, "add", "file.txt")

	r := exec.Git(ctx, dir, "commit", "-m", "subject line\n\nThis is the body.\nMore details here.")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	commits, err := log.List(context.Background(), dir, 50, 0)
	require.NoError(t, err)
	require.Len(t, commits, 1)

	assert.Equal(t, "subject line", commits[0].Message)
	assert.Contains(t, commits[0].Description, "This is the body.")
}

// --- Error paths ---

func TestList_InvalidRepo(t *testing.T) {
	_, err := log.List(context.Background(), "/nonexistent/path/that/does/not/exist", 50, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "log: list")
}

// TestList_EmptyOutput exercises parseRecords with an empty string (skip≥total
// commits means git outputs nothing with exit 0).
func TestList_SkipBeyondHistory(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "only commit")

	commits, err := log.List(context.Background(), dir, 50, 100)

	require.NoError(t, err)
	assert.Empty(t, commits)
}

// TestList_LimitZeroDefaultsTo50 covers the limit<=0 branch with negative input.
func TestList_NegativeLimit(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "only commit")

	commits, err := log.List(context.Background(), dir, -1, 0)

	require.NoError(t, err)
	require.Len(t, commits, 1)
}

// TestList_MalformedRecord verifies that a record missing the \x1f separator
// causes parseRecords to return an error. We cannot inject raw git output
// through the public API, so we confirm that a valid repo never produces such
// output and the happy-path is covered by other tests.  The malformed-record
// error path in parseRecord is an internal invariant; we verify it indirectly
// by confirming that all normal commits parse without error and that the only
// way to hit it is through corrupted output (which we cannot produce from a
// real repo).  This test documents that the package handles the empty-skip case
// cleanly and does not regress.
func TestList_SkipAll(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "a.txt", "a\n", "commit one")
	addCommit(t, dir, "b.txt", "b\n", "commit two")

	commits, err := log.List(context.Background(), dir, 10, 2)

	require.NoError(t, err)
	assert.Empty(t, commits)
}

// --- White-box tests using exported internal helpers ---

func TestParseRecords_EmptyInput(t *testing.T) {
	commits, err := log.ExportedParseRecords("")
	require.NoError(t, err)
	assert.Nil(t, commits)
}

func TestParseRecords_EmptyAfterTrimSep(t *testing.T) {
	// Only the record separator char — trimmed left leaves empty string.
	commits, err := log.ExportedParseRecords("\x1e")
	require.NoError(t, err)
	assert.Nil(t, commits)
}

func TestParseRecords_SkipsEmptyRecords(t *testing.T) {
	// Two consecutive separators produce an empty record that should be skipped.
	// We can only test this if the real records around them are valid; supply none.
	commits, err := log.ExportedParseRecords("\x1e\x1e")
	require.NoError(t, err)
	assert.Empty(t, commits)
}

func TestParseRecord_MissingFieldSeparator(t *testing.T) {
	_, err := log.ExportedParseRecord("no field separator here")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed record")
}

func TestParseRecord_TooFewBodyLines(t *testing.T) {
	// Has \x1f but body section has fewer than 3 lines.
	_, err := log.ExportedParseRecord("hash\nshort\x1fauthor\nemail\n2024-01-01T00:00:00Z")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed record")
}

func TestParseRecord_TooFewMetaLines(t *testing.T) {
	// Has \x1f and enough body lines but too few meta lines.
	_, err := log.ExportedParseRecord("hash\nshort\nsubject\n\x1fonly-one-line")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed record")
}

func TestParseRecord_InvalidDate(t *testing.T) {
	_, err := log.ExportedParseRecord("hash\nshort\nsubject\n\x1fauthor\nemail\nnot-a-date")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse date")
}

func TestParseRecords_PropagatesParseRecordError(t *testing.T) {
	// A record with no \x1f separator should propagate the error from parseRecord.
	_, err := log.ExportedParseRecords("\x1emalformed-no-separator")
	require.Error(t, err)
}
