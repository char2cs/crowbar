package exec_test

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/perf"
)

func writeAndCommit(
	t *testing.T,
	dir string,
	name string,
	content string,
) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	add := exec.Git(ctx, dir, "add", name)
	require.Equal(t, 0, add.ExitCode, add.Stderr)
	commit := exec.Git(ctx, dir, "commit", "-q", "-m", "c")
	require.Equal(t, 0, commit.ExitCode, commit.Stderr)
}

func TestGitStream_StreamsStdout(t *testing.T) {
	dir := initRepo(t)

	r, wait, err := exec.GitStream(context.Background(), dir, "status", "--porcelain=v2", "--branch")
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	var lines int
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lines++
	}
	require.NoError(t, sc.Err())
	require.NoError(t, wait())
	assert.Positive(t, lines)
}

func TestGitStream_WaitReportsNonZeroExit(t *testing.T) {
	dir := t.TempDir() // not a git repo

	r, wait, err := exec.GitStream(context.Background(), dir, "log")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()

	err = wait()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log")
}

// The point of streaming is that a consumer can stop early without the whole
// output ever being materialised. Closing the reader must not deadlock.
//
// The history is sized so `log -p` produces far more than a pipe buffer holds:
// after the single 64-byte read below, git is parked in write() with a full
// pipe, which is the state Close has to break out of. A history small enough to
// fit in the buffer would let git exit on its own and prove nothing.
func TestGitStream_CloseBeforeEOFDoesNotBlock(t *testing.T) {
	dir := initRepo(t)
	for i := range 30 {
		line := strings.Repeat("x", 60) + string(rune('a'+i%26)) + "\n"
		writeAndCommit(t, dir, "f.txt", strings.Repeat(line, 300))
	}

	r, wait, err := exec.GitStream(context.Background(), dir, "log", "-p")
	require.NoError(t, err)

	buf := make([]byte, 64)
	_, _ = r.Read(buf)
	require.NoError(t, r.Close())
	_ = wait() // may report a signal kill; must return rather than hang
}

func TestGitStream_ContextCancelStopsProcess(t *testing.T) {
	dir := initRepo(t)
	ctx, cancel := context.WithCancel(context.Background())

	r, wait, err := exec.GitStream(ctx, dir, "log")
	require.NoError(t, err)
	cancel()
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
	_ = wait()
}

// A stream that finished normally is not a cancellation: closing the reader
// after EOF must leave wait's verdict untouched, or every fully-consumed stream
// would report a spurious error.
func TestGitStream_WaitAfterCloseAtEOFReportsSuccess(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "a.txt", "hello\n")

	r, wait, err := exec.GitStream(context.Background(), dir, "log", "--oneline")
	require.NoError(t, err)
	n, err := io.Copy(io.Discard, r)
	require.NoError(t, err)
	require.Positive(t, n)
	require.NoError(t, r.Close())

	assert.NoError(t, wait())
	assert.NoError(t, wait())
}

func TestGitStream_RecordsSampleNamedForSubcommand(t *testing.T) {
	dir := initRepo(t)
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

	r, wait, err := exec.GitStream(context.Background(), dir, "status", "--porcelain")
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, r)
	require.NoError(t, wait())
	require.NoError(t, r.Close())

	var names []string
	for _, s := range perf.Snapshot() {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "git.status")
}

func TestGit_ExpiredDeadlineIsClassifiedAsTimeout(t *testing.T) {
	dir := initRepo(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	r := exec.Git(ctx, dir, "status")

	require.NotEqual(t, 0, r.ExitCode)
	err := exec.RequireSuccess("status", r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestGit_CancelledContextIsNotReportedAsTimeout(t *testing.T) {
	dir := initRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := exec.Git(ctx, dir, "status")

	require.NotEqual(t, 0, r.ExitCode)
	assert.NotContains(t, r.Stderr, "timed out")
}
