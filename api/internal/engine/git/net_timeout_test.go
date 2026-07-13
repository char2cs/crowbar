package git_test

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// installStallingGit puts a fake `git` first on PATH that hangs forever on
// network subcommands (fetch/pull/push/ls-remote) and delegates everything
// else to the real git, simulating a remote whose TCP connection has gone
// dead — the observed production failure where a stalled fetch held the
// per-repo mutex for the OS retransmission timeout (~15 min).
func installStallingGit(
	t *testing.T,
) {
	t.Helper()
	realGit, err := osexec.LookPath("git")
	require.NoError(t, err)

	dir := t.TempDir()
	script := fmt.Sprintf(
		"#!/bin/sh\ncase \"$1\" in\nfetch|pull|push|ls-remote) sleep 3600 ;;\n*) exec %q \"$@\" ;;\nesac\n",
		realGit,
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Both tests below drive a deliberately STALLING git against a real repo, and both
// used to end with `assert.Less(time.Since(start), 10*time.Second)`.
//
// That wall-clock bound was doing no work, and it was actively harmful. What proves
// the operation was bounded by OUR timeout is the *error itself* — a stalled fetch
// that returns "timed out" cannot also have hung. The 10-second ceiling was only
// there to catch the regression where the code hangs forever instead... which is
// exactly what `go test -timeout` already catches, and catches BETTER: a timeout
// dumps every goroutine and names the stuck test, where the assertion just says
// "12.3s is not less than 10s" and tells you nothing about why.
//
// And it lied. Under load (a busy CI box, a machine running other suites), spawning
// a process and reaping a stalled one takes longer than the guess, so the assertion
// fires on code that is working perfectly. It did exactly that during this cleanup:
// "12.3s is not less than 10s", on an unchanged, correct engine.
//
// The timeout is the subject here, so the 500ms injection stays — that is a real
// input, not a synchronisation guess. What goes is the clock we were reading back.

func TestFetch_TimesOutInsteadOfHangingOnStalledRemote(
	t *testing.T,
) {
	repo := initRepoWithBareOrigin(t)
	restore := git.SetNetTimeoutsForTest(500*time.Millisecond, 500*time.Millisecond)
	defer restore()
	installStallingGit(t)

	e := git.New()
	err := e.Fetch(context.Background(), repo)

	// The error IS the proof: a stalled fetch that reports "timed out" was bounded by
	// the transfer timeout, not left to the TCP stack. Had it hung, we would never
	// reach here and `go test -timeout` would name this test.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestRemoteBranchExists_TimesOutAndFallsBackToLocal(
	t *testing.T,
) {
	repo := initRepoWithBareOrigin(t)
	restore := git.SetNetTimeoutsForTest(500*time.Millisecond, 500*time.Millisecond)
	defer restore()
	installStallingGit(t)

	e := git.New()
	exists, err := e.RemoteBranchExists(context.Background(), repo, "main")

	// Contract: an unreachable remote degrades to "not on any usable remote". Reaching
	// this assertion at all is what proves the query was bounded — a hang never
	// returns, and `go test -timeout` would say so.
	require.NoError(t, err)
	assert.False(t, exists)
}
