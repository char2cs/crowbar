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

func TestFetch_TimesOutInsteadOfHangingOnStalledRemote(
	t *testing.T,
) {
	repo := initRepoWithBareOrigin(t)
	restore := git.SetNetTimeoutsForTest(500*time.Millisecond, 500*time.Millisecond)
	defer restore()
	installStallingGit(t)

	e := git.New()
	start := time.Now()
	err := e.Fetch(context.Background(), repo)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(
		t, time.Since(start), 10*time.Second,
		"a stalled fetch must be bounded by the transfer timeout, not the TCP stack",
	)
}

func TestRemoteBranchExists_TimesOutAndFallsBackToLocal(
	t *testing.T,
) {
	repo := initRepoWithBareOrigin(t)
	restore := git.SetNetTimeoutsForTest(500*time.Millisecond, 500*time.Millisecond)
	defer restore()
	installStallingGit(t)

	e := git.New()
	start := time.Now()
	exists, err := e.RemoteBranchExists(context.Background(), repo, "main")

	// Contract: an unreachable remote degrades to "not on any usable remote".
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Less(
		t, time.Since(start), 10*time.Second,
		"a stalled ls-remote must be bounded by the query timeout",
	)
}
