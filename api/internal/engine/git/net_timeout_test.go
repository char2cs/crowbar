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

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/git"
)

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
	binpath.InvalidateGit()
	t.Cleanup(binpath.InvalidateGit)
}

func TestFetch_TimesOutInsteadOfHangingOnStalledRemote(
	t *testing.T,
) {
	repo := initRepoWithBareOrigin(t)
	restore := git.SetNetTimeoutsForTest(500*time.Millisecond, 500*time.Millisecond)
	defer restore()
	installStallingGit(t)

	e := git.New()
	err := e.Fetch(context.Background(), repo)

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

	require.NoError(t, err)
	assert.False(t, exists)
}
