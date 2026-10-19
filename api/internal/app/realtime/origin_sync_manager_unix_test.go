//go:build unix

package realtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// heldSmudge is a smudge filter that, once git is checking its file out, reports
// on ready and waits for a line on release. It drops stderr so a git that dies
// under it is not kept waiting on the pipe.
const heldSmudge = `#!/bin/sh
exec 2>/dev/null 3<>'%RELEASE%'
echo held > '%READY%'
read _ <&3
exec cat
`

// mergeCtxRecorder is a real engine that records the context every fast-forward
// runs under.
type mergeCtxRecorder struct {
	OriginFetcher
	ctxs chan context.Context
}

func (r mergeCtxRecorder) MergeFFOnly(
	ctx context.Context,
	repoPath string,
	ref string,
) error {
	r.ctxs <- ctx
	return r.OriginFetcher.MergeFFOnly(ctx, repoPath, ref)
}

func gitOut(
	t *testing.T,
	dir string,
	args ...string,
) string {
	t.Helper()
	args = append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	out, err := exec.CommandContext(t.Context(), "git", args...).CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func commitFile(
	t *testing.T,
	repo string,
	name string,
	content string,
) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600))
	gitOut(t, repo, "add", name)
	gitOut(t, repo, "commit", "-q", "-m", name)
}

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

// heldLockedRoot clones origin into a locked root whose next fast-forward of
// "held" blocks in the smudge filter, and returns the root and the two FIFOs.
func heldLockedRoot(
	t *testing.T,
	origin string,
) (root string, ready string, release string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "root")
	gitOut(t, origin, "clone", "-q", origin, root)
	fifos := t.TempDir()
	ready, release = filepath.Join(fifos, "ready"), filepath.Join(fifos, "release")
	require.NoError(t, syscall.Mkfifo(ready, 0o600))
	require.NoError(t, syscall.Mkfifo(release, 0o600))
	t.Cleanup(func() { freeFIFO(t, ready); freeFIFO(t, release) })
	filter := filepath.Join(fifos, "smudge")
	script := strings.NewReplacer("%READY%", ready, "%RELEASE%", release).Replace(heldSmudge)
	require.NoError(t, os.WriteFile(filter, []byte(script), 0o755))
	gitOut(t, root, "config", "filter.hold.smudge", filter)
	return root, ready, release
}

// A cancel never interrupts a fast-forward in progress: killed mid-checkout, git
// leaves the locked root half-advanced and refuses every later fast-forward.
func TestOriginSyncManager_CancelDuringTheFastForwardLetsItFinish(t *testing.T) {
	origin := t.TempDir()
	gitOut(t, origin, "init", "-q", "-b", "develop")
	commitFile(t, origin, ".gitattributes", "held filter=hold\n")
	commitFile(t, origin, "held", "v1\n")
	root, ready, release := heldLockedRoot(t, origin)
	commitFile(t, origin, "held", "v2\n")
	w := newFakeOriginWorkspaces()
	ws := lockedRootWorkspace("w1")
	ws.WorktreePath = root
	w.set(ws)
	g := mergeCtxRecorder{OriginFetcher: enginegit.New(), ctxs: make(chan context.Context, 1)}
	m := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, g)
	d := driveOriginSyncs(m)
	t.Cleanup(m.StopAll)
	held := make(chan error, 1)
	go func() { _, err := os.ReadFile(ready); held <- err }()

	m.Acquire("w1")
	select {
	case err := <-held:
		require.NoError(t, err)
	case <-d.cycles:
		require.FailNow(t, "the open sync ended before its fast-forward reached the filter")
	}
	mergeCtx := <-g.ctxs
	m.Release("w1")
	require.NoError(t, mergeCtx.Err(), "the cancel reached the fast-forward in progress")
	freeFIFO(t, release)
	m.waitRunnersForTest()
	assert.Empty(t, gitOut(t, root, "status", "--porcelain"), "the fast-forward finished cleanly")
	assert.Equal(t, gitOut(t, origin, "rev-parse", "HEAD"), gitOut(t, root, "rev-parse", "HEAD"))

	commitFile(t, origin, "plain", "v3\n")
	next := NewOriginSyncManager(context.Background(), testOriginSyncInterval, w, enginegit.New())
	nd := driveOriginSyncs(next)
	t.Cleanup(next.StopAll)
	next.Acquire("w1")
	<-nd.cycles
	assert.Equal(t, gitOut(t, origin, "rev-parse", "HEAD"), gitOut(t, root, "rev-parse", "HEAD"),
		"the next open fast-forwards the root again")
}
