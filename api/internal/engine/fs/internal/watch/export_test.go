package watch

// This file is compiled only into the watch package's test binary. It exposes
// the watcher's internal synchronisation seams to the external watch_test
// package so that tests can block on real signals — the loop's exit, a flushed
// debounce, a hand-fired timer — instead of guessing with a clock.

// GitTimer is the trailing git-recompute debounce timer. A test substitutes an
// implementation it fires by hand (see SetGitTimerForTest) so that "N bursts
// coalesce into exactly one recompute" can be asserted exactly, with no wall
// clock involved.
type GitTimer = debounceTimer

// SetGitTimerForTest replaces the git-recompute debounce timer. It must be
// called before Start: the `go w.loop` inside Start is what publishes the field
// to the loop goroutine.
func (w *Watcher) SetGitTimerForTest(
	t GitTimer,
) {
	w.gitTimer = t
}

// SetOnGitRecomputeForTest installs a hook the loop goroutine invokes after each
// completed git recompute — including a recompute that fanOutGit short-circuits
// (mid-rebase/merge), which is otherwise invisible to the GitStatusProvider.
// It must be called before Start.
func (w *Watcher) SetOnGitRecomputeForTest(
	fn func(),
) {
	w.onGitRecompute = fn
}

// LoopDoneForTest returns a channel closed when the loop goroutine has exited.
func (w *Watcher) LoopDoneForTest() <-chan struct{} {
	return w.loopDone
}
