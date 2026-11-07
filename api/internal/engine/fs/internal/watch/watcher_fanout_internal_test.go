package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// ---------------------------------------------------------------------------
// fanOutGit — the diff base is a function of refs, so working-tree bursts reuse it
// ---------------------------------------------------------------------------

// baseCountingGit counts how often the watcher asks git for the diff base and
// the working-tree stat, and answers the base from a field the test can change.
type baseCountingGit struct {
	minimalGit
	mu           sync.Mutex
	bases, stats int
	base         gitdomain.DiffBase
}

func (g *baseCountingGit) ResolveDiffBase(
	_ context.Context,
	_ string,
	_ string,
) gitdomain.DiffBase {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.bases++
	return g.base
}

func (g *baseCountingGit) DiffStat(
	_ context.Context,
	_ string,
	_ string,
) (int, int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stats++
	return 0, 0, nil
}

func (g *baseCountingGit) setBase(b gitdomain.DiffBase) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.base = b
}

func (g *baseCountingGit) counts() (bases, stats int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.bases, g.stats
}

func TestFanOutGit_WorkingTreeBurstsReuseResolvedDiffBase(t *testing.T) {
	g := &baseCountingGit{}
	w := NewWatcher("ws-base", t.TempDir(), "main", g, &recordingDispatcher{})

	for range 3 {
		w.fanOutGit(context.Background())
	}

	bases, stats := g.counts()
	assert.Equal(t, 1, bases, "no ref moved, so the base is resolved once")
	assert.Equal(t, 3, stats, "the working-tree stat still follows every burst")
}

func TestFanOutGit_RefEventReResolvesDiffBase(t *testing.T) {
	dir := t.TempDir()
	g := &baseCountingGit{}
	rd := &recordingDispatcher{}
	w := NewWatcher("ws-base", dir, "main", g, rd)
	w.fanOutGit(context.Background())

	g.setBase(gitdomain.DiffBase{Commit: "abc", HasCommits: true})
	w.fanOutGit(context.Background())
	bases, _ := g.counts()
	require.Equal(t, 1, bases, "a burst with no ref event must not see the new base")

	w.handleOne(context.Background(), event{
		Name: filepath.Join(dir, ".git", "refs", "heads", "feature"),
		Op:   opWrite,
	})
	w.fanOutGit(context.Background())

	bases, _ = g.counts()
	assert.Equal(t, 2, bases, "a ref event makes the next recompute resolve again")
	got, ok := rd.lastSync()
	require.True(t, ok, "the changed HasCommits must be dispatched")
	assert.True(t, got.HasCommits)
}

// ---------------------------------------------------------------------------
// scheduleGitRecompute — wait for the burst to end, but never past the ceiling
// ---------------------------------------------------------------------------

// delayRecordingTimer is a debounceTimer that records how long each Reset asked
// to wait and never fires.
type delayRecordingTimer struct {
	delays []time.Duration
}

func (d *delayRecordingTimer) Reset(wait time.Duration) bool {
	d.delays = append(d.delays, wait)
	return true
}

func (d *delayRecordingTimer) Stop() bool { return true }

func (d *delayRecordingTimer) Chan() <-chan time.Time { return nil }

func (d *delayRecordingTimer) last() time.Duration { return d.delays[len(d.delays)-1] }

func TestScheduleGitRecompute_WaitsForQuietNeverPastMaxWait(t *testing.T) {
	clock := time.Unix(1_000, 0)
	timer := &delayRecordingTimer{}
	w := newBareWatcher(t.TempDir(), &recordingDispatcher{})
	w.gitTimer = timer
	w.now = func() time.Time { return clock }

	w.scheduleGitRecompute()
	assert.Equal(t, gitRecomputeQuiet, timer.last(), "a first change waits for the burst to end")

	clock = clock.Add(gitRecomputeQuiet / 2)
	w.scheduleGitRecompute()
	assert.Equal(t, gitRecomputeQuiet, timer.last(), "more changes push the recompute back")

	clock = clock.Add(gitRecomputeMaxWait - gitRecomputeQuiet/2 - gitRecomputeQuiet/4)
	w.scheduleGitRecompute()
	assert.Equal(t, gitRecomputeQuiet/4, timer.last(), "near the ceiling the wait shrinks to what is left")

	clock = clock.Add(gitRecomputeMaxWait)
	w.scheduleGitRecompute()
	assert.Zero(t, timer.last(), "a burst that never ends still recomputes once the ceiling passes")
}

func TestScheduleGitRecompute_CeilingRestartsAfterARecompute(t *testing.T) {
	clock := time.Unix(1_000, 0)
	timer := &delayRecordingTimer{}
	w := newBareWatcher(t.TempDir(), &recordingDispatcher{})
	w.gitTimer = timer
	w.now = func() time.Time { return clock }
	w.scheduleGitRecompute()
	clock = clock.Add(gitRecomputeMaxWait * 2)

	w.runGitRecompute(context.Background())
	w.scheduleGitRecompute()

	assert.Equal(t, gitRecomputeQuiet, timer.last(), "the ceiling counts from the first change after a recompute")
}

// ---------------------------------------------------------------------------
// fanOutGit — a status git cannot read is reported, and recovery is re-broadcast
// ---------------------------------------------------------------------------

// flakyGit fails ComputeStatus while broken is set.
type flakyGit struct {
	minimalGit
	broken bool
}

func (g *flakyGit) ComputeStatus(_ context.Context, _ string) (gitdomain.GitStatus, error) {
	if g.broken {
		return gitdomain.GitStatus{}, errors.New("not a git repository")
	}
	return gitdomain.GitStatus{Branch: "main"}, nil
}

func TestFanOutGit_UnreadableStatusIsReportedAndRecoveryReBroadcast(t *testing.T) {
	g := &flakyGit{}
	rd := &recordingDispatcher{}
	w := NewWatcher("ws-moved", t.TempDir(), "main", g, rd)
	ctx := context.Background()

	w.fanOutGit(ctx)
	g.broken = true
	w.fanOutGit(ctx)
	w.fanOutGit(ctx)
	assert.Equal(t, 1, rd.errors(), "a failed read is reported once, not once per recompute")

	g.broken = false
	w.fanOutGit(ctx)
	assert.Equal(t, 2, rd.count(), "the recovered status equals the last one sent and is still re-broadcast")
	g.broken = true
	w.fanOutGit(ctx)
	assert.Equal(t, 2, rd.errors(), "a failure after a recovery is news again")
}

// gitStatusOf runs a real git status, so a repo moved away fails as it does in production.
type gitStatusOf struct{ minimalGit }

func (gitStatusOf) ComputeStatus(ctx context.Context, repoPath string) (gitdomain.GitStatus, error) {
	if out, err := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain").CombinedOutput(); err != nil {
		return gitdomain.GitStatus{}, fmt.Errorf("git status: %w: %s", err, out)
	}
	return gitdomain.GitStatus{Branch: "feature"}, nil
}

// Moving the parent repo sends nothing into a linked worktree's tree or its watched git
// dirs (inotify follows the moved inodes), yet git can no longer read the worktree; the
// main worktree's own watcher must also see the repo come back to re-arm its tree.
func TestMovingTheParentRepoIsReportedAndMovingItBackRecovers(t *testing.T) {
	for _, watched := range []string{"linked worktree", "main worktree"} {
		t.Run(watched, func(t *testing.T) {
			worktree, commonDir := newLinkedWorktree(t)
			repo := filepath.Dir(commonDir)
			if watched == "main worktree" {
				worktree = repo
			}
			rd := &recordingDispatcher{
				statuses:   make(chan gitdomain.GitStatus, 16),
				unreadable: make(chan error, 16),
				files:      make(chan string, 64),
			}
			w := NewWatcher("ws-moved", worktree, "", &gitStatusOf{}, rd)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			t.Cleanup(w.Stop)
			require.NoError(t, w.Start(ctx))

			require.NoError(t, os.Rename(repo, repo+"-moved"))
			select {
			case <-rd.unreadable:
			case <-time.After(15 * time.Second):
				t.Fatal("moving the parent repo away was never reported")
			}

			require.NoError(t, os.Rename(repo+"-moved", repo))
			select {
			case <-rd.statuses:
			case <-time.After(15 * time.Second):
				t.Fatal("moving the parent repo back never re-broadcast the status")
			}
			require.NoError(t, os.WriteFile(filepath.Join(worktree, "after.txt"), []byte("x"), 0o600))
			rd.waitForFile("after.txt")
		})
	}
}

// The main worktree's parent directory is watched for that one entry; its other
// entries are someone else's and must neither surface nor recompute.
func TestAnchorParent_SiblingEntriesAreDropped(t *testing.T) {
	worktree, commonDir := newLinkedWorktree(t)
	rd := newRecordingDispatcher()
	w := NewWatcher("ws-anchor", worktree, "", &minimalGit{}, rd)

	sibling := filepath.Join(filepath.Dir(filepath.Dir(commonDir)), "unrelated")
	assert.False(t, w.handleOne(context.Background(), event{Name: sibling, Op: opWrite}))
	assert.True(t, w.handleOne(context.Background(), event{Name: filepath.Dir(commonDir), Op: opRename}),
		"the main worktree moving is a reason to recompute")
	assert.Empty(t, rd.files, "neither is a file of this workspace")
}
