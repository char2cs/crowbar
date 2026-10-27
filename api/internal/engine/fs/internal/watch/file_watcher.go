package watch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// FileWatcher follows one file, which need not exist yet, and reports each
// debounced change to it. It is single-use, like Watcher.
type FileWatcher struct {
	path string
	emit func(change domain.FileChangeType)

	mu       sync.Mutex
	cancel   context.CancelFunc
	started  bool
	stopped  bool
	loopDone chan struct{}
}

// NewFileWatcher builds a FileWatcher on path; emit runs on the watcher's own
// goroutine. Call Start to begin.
func NewFileWatcher(
	path string,
	emit func(change domain.FileChangeType),
) *FileWatcher {
	return &FileWatcher{path: path, emit: emit, loopDone: make(chan struct{})}
}

// Start arms the OS watch and returns once it is armed: a change made after
// Start returns is reported.
func (w *FileWatcher) Start(
	ctx context.Context,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	be, err := newBackend(ctx, watchSpec{tree: w.path, file: true})
	if err != nil {
		return fmt.Errorf("watch: follow %s: %w", w.path, err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return failStart(ErrAlreadyStarted, be)
	}
	if w.stopped {
		return failStart(context.Canceled, be)
	}
	w.started = true
	loopCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go w.loop(loopCtx, be)
	return nil
}

// Stop ends the watch and joins its loop, so no emit follows it. It is
// idempotent, and before Start it makes Start refuse.
func (w *FileWatcher) Stop() {
	w.mu.Lock()
	w.stopped = true
	cancel, started := w.cancel, w.started
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if started {
		<-w.loopDone
	}
}

func (w *FileWatcher) loop(
	ctx context.Context,
	be backend,
) {
	defer close(w.loopDone)
	defer safego.Recover("fs.watch.file")
	defer func() {
		if err := be.close(); err != nil {
			slog.Warn("watch: close file backend", "path", w.path, "err", err)
		}
	}()
	pending := newBurst()
	defer pending.stop()
	events, errs := be.events(), be.errors()
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return
			}
			// One file under whatever spelling the OS reports it.
			pending.add(w.path, evt.Op)
		case _, ok := <-errs:
			if !ok {
				errs = nil
			}
		case <-pending.ready():
			for _, evt := range pending.take() {
				w.emit(classifyChange(evt.Op))
			}
		case <-ctx.Done():
			return
		}
	}
}
