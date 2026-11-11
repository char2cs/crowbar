//go:build unix

package exec_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// holdRefLocksHook is a reference-transaction hook that, once git holds the ref
// locks, reports on ready and waits for a line on release. It drops stdout and
// stderr so a git that dies under it is not kept waiting on the pipes.
const holdRefLocksHook = `#!/bin/sh
[ "$1" = prepared ] || exit 0
exec >/dev/null 2>&1 3<>'%RELEASE%'
echo held > '%READY%'
read _ <&3
`

// freeFIFO lets go whoever waits on either end of path: opened read-write it
// never blocks, completes a waiting writer's open and hands a reader a line.
func freeFIFO(
	t *testing.T,
	path string,
) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	_, err = f.WriteString("\n")
	assert.NoError(t, err)
	assert.NoError(t, f.Close())
}

func lockFiles(
	t *testing.T,
	gitDir string,
) []string {
	t.Helper()
	var locks []string
	require.NoError(t, filepath.WalkDir(gitDir, func(path string, _ fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".lock") {
			locks = append(locks, path)
		}
		return err
	}))
	return locks
}

// A fetch cancelled while it holds its ref locks must get a signal git cleans
// up on (SIGTERM), not a SIGKILL that strands refs/...lock for every later fetch.
func TestGit_CancelledFetchLeavesNoRefLock(t *testing.T) {
	ctx := context.Background()
	src := initRepo(t)
	require.Equal(t, 0, exec.Git(ctx, src, "commit", "--allow-empty", "-m", "init").ExitCode)
	dst := initRepo(t)

	fifos := t.TempDir()
	ready, release := filepath.Join(fifos, "ready"), filepath.Join(fifos, "release")
	require.NoError(t, syscall.Mkfifo(ready, 0o600))
	require.NoError(t, syscall.Mkfifo(release, 0o600))
	t.Cleanup(func() { freeFIFO(t, ready); freeFIFO(t, release) })
	hooks := t.TempDir()
	hook := strings.NewReplacer("%READY%", ready, "%RELEASE%", release).Replace(holdRefLocksHook)
	require.NoError(t, os.WriteFile(filepath.Join(hooks, "reference-transaction"), []byte(hook), 0o755))
	require.Equal(t, 0, exec.Git(ctx, dst, "config", "core.hooksPath", hooks).ExitCode)

	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	fetched := make(chan exec.Result, 1)
	go func() { fetched <- exec.Git(fetchCtx, dst, "fetch", src, "main:refs/remotes/origin/main") }()
	held := make(chan error, 1)
	go func() { _, err := os.ReadFile(ready); held <- err }()
	select {
	case err := <-held:
		require.NoError(t, err)
	case r := <-fetched:
		require.FailNow(t, "the fetch ended before its hook held the ref locks", "%+v", r)
	}
	gitDir := filepath.Join(dst, ".git")
	require.NotEmpty(t, lockFiles(t, gitDir), "the cancel must land while git holds a ref lock")

	cancel()
	r := <-fetched
	assert.NotEqual(t, 0, r.ExitCode)
	assert.Empty(t, lockFiles(t, gitDir), "a cancelled git removes its own lockfiles")
}
