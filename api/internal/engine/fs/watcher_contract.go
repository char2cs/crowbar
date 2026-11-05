package fs

import "github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"

// Watcher is the per-workspace file watcher returned by Engine.NewWatcher. The
// caller owns its Start (blocking) and Stop (idempotent) lifecycle.
type Watcher = watch.Watcher

// ErrWatchLimit is a FileWatcher arm the OS refused for want of watch resources.
var ErrWatchLimit = watch.ErrWatchLimit

// FileWatcher follows one file for Engine.NewFileWatcher; Start returns once
// the OS watch is armed.
type FileWatcher = watch.FileWatcher

// Dispatcher receives the watcher fan-out callbacks (OnFileChange, OnGitStatus,
// OnSyncWorkingTreeState). The delivery layer implements it to broadcast and to
// issue the SyncWorkingTreeState command.
type Dispatcher = watch.Dispatcher

// SyncInput carries the recomputed working-tree summary delivered to
// Dispatcher.OnSyncWorkingTreeState.
type SyncInput = watch.SyncInput

// GitStatusProvider recomputes GitStatus from live git state for the watcher.
// The delivery layer adapts the git engine to it.
type GitStatusProvider = watch.GitStatusProvider
