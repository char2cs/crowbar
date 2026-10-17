package watch

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
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

	w.handleOne(context.Background(), fsnotify.Event{
		Name: filepath.Join(dir, ".git", "refs", "heads", "feature"),
		Op:   fsnotify.Write,
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
