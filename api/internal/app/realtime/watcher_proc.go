package realtime

import "context"

// watcherProc is the lifecycle surface the WatcherManager drives. *watch.Watcher
// satisfies it; tests inject a fake to assert lazy start/stop without real fsnotify.
type watcherProc interface {
	// Start begins watching under ctx and returns once the watch runs. ctx bounds
	// the whole watch, so cancelling it also cuts a slow start short.
	Start(
		ctx context.Context,
	) error
	// Stop ends a running watch and waits for it (a Start in flight is the manager's to join).
	Stop()
}

// watcherFactory builds a watcherProc for a workspace. It resolves the
// worktree path and forkPointSha, wires the dispatcher and git provider, and
// returns the unstarted watcher.
type watcherFactory func(
	ctx context.Context,
	wsID string,
) (watcherProc, error)
