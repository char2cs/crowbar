package watch

type GitTimer = debounceTimer

func (w *Watcher) SetGitTimerForTest(
	t GitTimer,
) {
	w.gitTimer = t
}

func (w *Watcher) SetOnGitRecomputeForTest(
	fn func(),
) {
	w.onGitRecompute = fn
}

func (w *Watcher) LoopDoneForTest() <-chan struct{} {
	return w.loopDone
}
