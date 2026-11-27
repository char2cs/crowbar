package fs

import "github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"

type Watcher = watch.Watcher

var ErrWatchLimit = watch.ErrWatchLimit

type FileWatcher = watch.FileWatcher

type Dispatcher = watch.Dispatcher

type SyncInput = watch.SyncInput

type GitStatusProvider = watch.GitStatusProvider
