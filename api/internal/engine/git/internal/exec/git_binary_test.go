package exec_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func trimNewline(
	s string,
) string {
	return strings.TrimRight(s, "\r\n")
}

// writeStubGit is a stand-in git that runs script whatever argv it is handed, so
// a test can tell from the output — or from a side effect the script leaves
// behind — which file was exec'd and how often.
func writeStubGit(
	t *testing.T,
	script string,
) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "git")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755))
	return path
}

func readFile(
	t *testing.T,
	path string,
) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// vanishedResolver models the exact production sequence for a git binary that
// disappeared under a running daemon: the cached path no longer exists, and the
// re-resolution behind RecoverGit lands on the bare name — binpath's own final
// fallback, which the OS then finds on PATH.
type vanishedResolver struct {
	current  atomic.Value
	recovers atomic.Int64
}

func newVanishedResolver(
	t *testing.T,
) *vanishedResolver {
	t.Helper()
	r := &vanishedResolver{}
	r.current.Store(filepath.Join(t.TempDir(), "usr", "bin", "git"))
	return r
}

func (r *vanishedResolver) bin() string {
	return r.current.Load().(string)
}

func (r *vanishedResolver) recover() bool {
	r.recovers.Add(1)
	r.current.Store("git")
	return true
}

// TestGit_FallsBackToPathWhenTheResolvedBinaryIsGone is the safety proof for
// the whole change. Resolution is forced to hand back a path that does not
// exist; the git call must still succeed, through PATH, with the right answer.
func TestGit_FallsBackToPathWhenTheResolvedBinaryIsGone(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "a.txt", "one\n")
	resolver := newVanishedResolver(t)
	defer exec.SetGitResolverForTest(resolver.bin, resolver.recover)()

	r := exec.Git(context.Background(), dir, "rev-parse", "HEAD")

	require.Equal(t, 0, r.ExitCode, "stderr: %s", r.Stderr)
	assert.Len(t, trimNewline(r.Stdout), 40)
	assert.Equal(t, int64(1), resolver.recovers.Load(), "recovery must be attempted exactly once")
}

// TestGitStream_FallsBackToPathWhenTheResolvedBinaryIsGone covers the streaming
// seam, which builds its pipes before starting and therefore has to rebuild the
// whole invocation rather than retry a started one.
func TestGitStream_FallsBackToPathWhenTheResolvedBinaryIsGone(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "a.txt", "one\n")
	resolver := newVanishedResolver(t)
	defer exec.SetGitResolverForTest(resolver.bin, resolver.recover)()

	stream, wait, err := exec.GitStream(context.Background(), dir, "rev-parse", "HEAD")
	require.NoError(t, err)
	defer stream.Close()

	out, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, wait())

	assert.Len(t, trimNewline(string(out)), 40)
	assert.Equal(t, int64(1), resolver.recovers.Load())
}

// TestGit_OrdinaryFailureNeverTouchesTheResolver keeps the recovery path off
// the ordinary error path. A git command that ran and said no is not evidence
// that anything is wrong with the binary, and re-resolving on it would put a
// second spawn behind every failed call in the daemon.
func TestGit_OrdinaryFailureNeverTouchesTheResolver(t *testing.T) {
	dir := initRepo(t)
	var recovers atomic.Int64
	defer exec.SetGitResolverForTest(binpath.Git, func() bool {
		recovers.Add(1)
		return true
	})()

	r := exec.Git(context.Background(), dir, "rev-parse", "--verify", "refs/heads/nope")

	require.NotEqual(t, 0, r.ExitCode)
	assert.Zero(t, recovers.Load(), "a non-zero exit is git's answer, not a broken binary")
}

// TestGit_MissingWorkingDirectoryIsNotAMissingBinary pins the distinction the
// stat-driven recovery exists to make. A deleted repo directory fails at
// fork/exec with the same "no such file or directory" as a deleted binary, and
// must not invalidate a perfectly good resolution.
func TestGit_MissingWorkingDirectoryIsNotAMissingBinary(t *testing.T) {
	good := initRepo(t)
	writeAndCommit(t, good, "a.txt", "one\n")
	gone := filepath.Join(t.TempDir(), "deleted-repo")
	before := binpath.Git()

	failed := exec.Git(context.Background(), gone, "rev-parse", "HEAD")
	require.NotEqual(t, 0, failed.ExitCode)

	assert.Equal(t, before, binpath.Git(), "the resolution must survive a missing repo dir")
	assert.Equal(t, 0, exec.Git(context.Background(), good, "rev-parse", "HEAD").ExitCode)
}

// TestGit_ACommandThatRanIsNeverRunTwice is the guard on the retry, and the
// reason its predicate is cmd.Process rather than cmd.ProcessState. Retrying is
// only ever safe for an invocation that did not start; re-running one that did
// would run a `commit` or a `worktree add` a second time. The stub records every
// execution and exits non-zero, and the resolver offers a recovery on every
// failure — so a retry gated on anything but "no process exists" shows up here
// as two recorded runs.
func TestGit_ACommandThatRanIsNeverRunTwice(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "runs")
	stub := writeStubGit(t, "echo ran >> "+ledger+"\nexit 3\n")
	var recovers atomic.Int64
	defer exec.SetGitResolverForTest(
		func() string { return stub },
		func() bool {
			recovers.Add(1)
			return true
		},
	)()

	r := exec.Git(context.Background(), t.TempDir(), "commit", "-m", "x")

	require.Equal(t, 3, r.ExitCode)
	assert.Zero(t, recovers.Load(), "a command that ran must never reach the resolver")
	assert.Equal(t, "ran", trimNewline(readFile(t, ledger)), "the command must have run exactly once")
}

// TestGit_UsesTheResolvedBinary proves the seam is actually wired: point it at
// a stand-in and the stand-in is what runs.
func TestGit_UsesTheResolvedBinary(t *testing.T) {
	stub := writeStubGit(t, "echo stub-git-ran")
	defer exec.SetGitResolverForTest(
		func() string { return stub },
		func() bool { return false },
	)()

	r := exec.Git(context.Background(), t.TempDir(), "rev-parse", "HEAD")

	require.Equal(t, 0, r.ExitCode, "stderr: %s", r.Stderr)
	assert.Equal(t, "stub-git-ran", trimNewline(r.Stdout))
}
