package v0

import "context"

// watcherProc is the lifecycle surface the WatcherManager drives: a blocking
// Start and an idempotent Stop. *watch.Watcher satisfies it; tests inject a
// fake to assert lazy start/stop without real fsnotify.
type watcherProc interface {
	// Start begins watching and blocks until ctx is cancelled or Stop is called.
	Start(
		ctx context.Context,
	) error
	// Stop tears the watcher down. It is idempotent.
	Stop()
}

// watcherFactory builds a watcherProc for a workspace. It resolves the
// worktree path and forkPointSha, wires the dispatcher and git provider, and
// returns the unstarted watcher.
type watcherFactory func(
	ctx context.Context,
	wsID string,
) (watcherProc, error)
