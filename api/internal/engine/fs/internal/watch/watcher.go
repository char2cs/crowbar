package watch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch/ignore"
)

const debounceDuration = 100 * time.Millisecond

const (
	gitRecomputeQuiet   = 2 * time.Second
	gitRecomputeMaxWait = 10 * time.Second
)

var ErrAlreadyStarted = errors.New("watch: already started")

type debounceTimer interface {
	Reset(d time.Duration) bool
	Stop() bool
	Chan() <-chan time.Time
}

type realTimer struct {
	t *time.Timer
}

func newStoppedTimer(
	d time.Duration,
) *realTimer {
	t := time.NewTimer(d)
	if !t.Stop() {
		<-t.C
	}
	return &realTimer{t: t}
}

func (r *realTimer) Reset(
	d time.Duration,
) bool {
	return r.t.Reset(d)
}

func (r *realTimer) Stop() bool {
	return r.t.Stop()
}

func (r *realTimer) Chan() <-chan time.Time {
	return r.t.C
}

type Watcher struct {
	wsID       string
	repoPath   string
	base       string
	git        GitStatusProvider
	dispatcher Dispatcher
	ignore     ignore.Matcher

	be          backend
	mu          sync.Mutex
	cancelLoop  context.CancelFunc
	started     bool
	stopped     bool
	beCloseOnce sync.Once

	gitDirOnce     sync.Once
	resolvedGitDir string

	commonDirOnce     sync.Once
	resolvedCommonDir string

	prevAdded    int
	prevDeleted  int
	prevConflict bool
	prevCommits  bool

	prevStatus    gitdomain.GitStatus
	prevStatusSet bool
	unreadable    bool

	diffBase *gitdomain.DiffBase

	gitTimer        debounceTimer
	gitPending      bool
	gitPendingSince time.Time

	now func() time.Time

	loopDone chan struct{}

	onGitRecompute func()
}

func NewWatcher(
	wsID string,
	repoPath string,
	base string,
	git GitStatusProvider,
	dispatcher Dispatcher,
) *Watcher {
	return &Watcher{
		wsID:       wsID,
		repoPath:   repoPath,
		base:       base,
		git:        git,
		dispatcher: dispatcher,
		ignore:     ignore.NewMatcher(repoPath),
		loopDone:   make(chan struct{}),
		gitTimer:   newStoppedTimer(gitRecomputeQuiet),
		now:        time.Now,
	}
}

func (w *Watcher) Start(
	ctx context.Context,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	be, err := newBackend(ctx, w.spec())
	if err != nil {
		return fmt.Errorf("watch: start: %w", err)
	}

	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return failStart(ErrAlreadyStarted, be)
	}
	if w.stopped {
		w.mu.Unlock()
		return failStart(context.Canceled, be)
	}
	w.started = true
	w.be = be
	loopCtx, cancel := context.WithCancel(ctx)
	w.cancelLoop = cancel
	w.mu.Unlock()

	go w.loop(loopCtx)
	return nil
}

func failStart(
	reason error,
	be backend,
) error {
	if err := be.close(); err != nil {
		return errors.Join(reason, err)
	}
	return reason
}

func (w *Watcher) spec() watchSpec {
	return watchSpec{
		tree:     w.repoPath,
		skip:     w.skipDir,
		gitDirs:  w.gitWatchDirs(),
		refTrees: w.gitRefRoots(),
		anchors:  w.anchors(),
	}
}

func (w *Watcher) closeBackend() {
	w.beCloseOnce.Do(func() {
		w.mu.Lock()
		be := w.be
		w.mu.Unlock()
		if be != nil {
			if err := be.close(); err != nil {
				slog.Warn("watch: close backend", "workspace", w.wsID, "err", err)
			}
		}
	})
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	w.stopped = true
	cancel, started := w.cancelLoop, w.started
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if started {
		<-w.loopDone
	}
}

func (w *Watcher) loop(
	ctx context.Context,
) {
	defer close(w.loopDone)
	defer safego.Recover("fs.watch.loop")
	defer w.closeBackend()

	pending := newBurst()
	defer pending.stop()
	events, errs := w.be.events(), w.be.errors()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return
			}
			pending.add(evt.Name, evt.Op)
		case <-errs:
		case <-pending.ready():
			w.handleBurst(ctx, pending.take())
		case <-w.gitTimer.Chan():
			w.runGitRecompute(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) handleBurst(
	ctx context.Context,
	events []event,
) {
	relevant := false
	for _, evt := range events {
		if w.handleOne(ctx, evt) {
			relevant = true
		}
	}
	if relevant {
		w.scheduleGitRecompute()
	}
}

func (w *Watcher) handleOne(
	ctx context.Context,
	evt event,
) bool {
	if evt.Op&opCreate != 0 {
		w.be.created(ctx, evt.Name)
	}
	if evt.Op&opRemove != 0 {
		w.be.removed(evt.Name)
	}
	if isAnchor, ok := w.anchorEntry(evt.Name); ok {
		if isAnchor {
			w.diffBase = nil
		}
		return isAnchor
	}
	isRef := w.isGitRefPath(evt.Name)
	if isRef {
		w.diffBase = nil
	}
	if w.shouldIgnore(evt.Name) {
		w.maybeHandleGitRef(ctx, evt)
		return isRef
	}

	w.dispatcher.OnFileChange(ctx, domain.FileChangeEvent{
		Type:      classifyChange(evt.Op),
		Authority: domain.DocumentAuthorityWorkspace,
		Path:      w.relPath(evt.Name),
		WsID:      w.wsID,
	})
	return true
}

func (w *Watcher) skipDir(
	path string,
) bool {
	return filepath.Base(path) == ".git" || w.ignore.Match(path)
}

func (w *Watcher) shouldIgnore(
	path string,
) bool {
	rel := w.relPath(path)
	if !isUnder(rel, ".git") {
		return w.isGitInternal(path)
	}
	isHead := rel == filepath.Join(".git", "HEAD")
	isRefs := isUnder(rel, filepath.Join(".git", "refs"))
	isPackedRefs := rel == filepath.Join(".git", "packed-refs")
	return !isHead && !isRefs && !isPackedRefs
}

func (w *Watcher) relPath(
	absPath string,
) string {
	rel, err := filepath.Rel(w.repoPath, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

func classifyChange(
	o op,
) domain.FileChangeType {
	if o&opCreate != 0 {
		return domain.FileChangeCreated
	}
	if o&opRemove != 0 {
		return domain.FileChangeDeleted
	}
	if o&opRename != 0 {
		return domain.FileChangeRenamed
	}
	return domain.FileChangeModified
}
