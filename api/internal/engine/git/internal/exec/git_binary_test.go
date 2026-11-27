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

func TestGitStream_FallsBackToPathWhenTheResolvedBinaryIsGone(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "a.txt", "one\n")
	resolver := newVanishedResolver(t)
	defer exec.SetGitResolverForTest(resolver.bin, resolver.recover)()

	stream, wait, err := exec.GitStream(context.Background(), dir, "rev-parse", "HEAD")
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	out, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, wait())

	assert.Len(t, trimNewline(string(out)), 40)
	assert.Equal(t, int64(1), resolver.recovers.Load())
}

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
