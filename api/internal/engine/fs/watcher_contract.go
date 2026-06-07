package fs

import "github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"

// Watcher is the per-workspace file watcher returned by Engine.NewWatcher. The
// caller owns its Start (blocking) and Stop (idempotent) lifecycle (05 §5,
// 03 §6).
type Watcher = watch.Watcher

// Dispatcher receives the watcher fan-out callbacks (OnFileChange, OnGitStatus,
// OnSyncWorkingTreeState). The delivery layer implements it to broadcast and to
// issue the SyncWorkingTreeState command (03 §5).
type Dispatcher = watch.Dispatcher

// SyncInput carries the recomputed working-tree summary delivered to
// Dispatcher.OnSyncWorkingTreeState (00 §5.3, 03 §5).
type SyncInput = watch.SyncInput

// GitStatusProvider recomputes GitStatus from live git state for the watcher.
// The delivery layer adapts the git engine to it (05 §5).
type GitStatusProvider = watch.GitStatusProvider
