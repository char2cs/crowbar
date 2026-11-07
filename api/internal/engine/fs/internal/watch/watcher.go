// Package watch watches a workspace recursively (inotify on Linux, FSEvents on
// darwin) and runs the change fan-out pipeline.
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

// A git recompute waits for changes to stop for gitRecomputeQuiet (bounded by gitRecomputeMaxWait), so
// a burst costs one recompute, and linked worktrees sharing one .git don't all shell out on one ref update.
const (
	gitRecomputeQuiet   = 2 * time.Second
	gitRecomputeMaxWait = 10 * time.Second
)

// ErrAlreadyStarted is returned by a second Start: a Watcher is single-use, and re-starting would leak
// the first backend and loop goroutine.
var ErrAlreadyStarted = errors.New("watch: already started")

// debounceTimer is the git-recompute debounce timer, an interface so tests can fire it by hand and
// prove coalescing without racing a wall clock. Production uses realTimer.
type debounceTimer interface {
	Reset(d time.Duration) bool
	Stop() bool
	Chan() <-chan time.Time
}

// realTimer is the production debounceTimer, backed by a *time.Timer.
type realTimer struct {
	t *time.Timer
}

// newStoppedTimer returns a realTimer that is armed for d but immediately
// stopped and drained, so it only ever fires after an explicit Reset.
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

// Watcher watches a workspace's repo root and fans out every debounced event
// to a Dispatcher. It self-manages recursive watching for subdirectories.
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
	// unreadable is set while recomputes fail, so a failure streak is reported once.
	unreadable bool

	// diffBase is the resolved base of the last recompute, kept until a ref
	// event says refs moved: it is a function of refs alone, so a working-tree
	// burst never changes it. Only the loop goroutine touches it.
	diffBase *gitdomain.DiffBase

	// gitTimer is the trailing git-recompute debounce, created stopped, armed by scheduleGitRecompute and
	// drained only by loop. gitPending (guarded by mu) marks a scheduled, unrun recompute.
	gitTimer   debounceTimer
	gitPending bool
	// gitPendingSince is when the oldest unrun change scheduled the recompute;
	// the max wait counts from it (guarded by mu).
	gitPendingSince time.Time

	// now is the clock the max wait reads; a test injects its own.
	now func() time.Time

	// loopDone is closed when the loop goroutine returns: the real signal that
	// the watcher has finished tearing down, which Stop joins on.
	loopDone chan struct{}

	// onGitRecompute, test-only, is invoked by the loop after every git recompute (short-circuited ones
	// too). Set before Start so `go w.loop` orders it; lets tests wait on the flush instead of sleeping.
	onGitRecompute func()
}

// NewWatcher builds a Watcher but does not start it. Call Start to begin.
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

// Start watches the repo root recursively until ctx is cancelled or Stop. Each backend is closed exactly
// once whatever the cancel/stop order; a second Start returns ErrAlreadyStarted, closing the new backend.
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

// failStart closes the backend Start built but will not use and returns why Start refused;
// a close error rides along, and the reason keeps its identity when closing succeeds.
func failStart(
	reason error,
	be backend,
) error {
	if err := be.close(); err != nil {
		return errors.Join(reason, err)
	}
	return reason
}

// spec is what the watcher observes. The git dirs and ref trees are resolved,
// never joined onto repoPath/.git: in a linked worktree that is a gitlink FILE,
// and a ref-only change (a bare fetch) touches no working-tree file.
func (w *Watcher) spec() watchSpec {
	return watchSpec{
		tree:     w.repoPath,
		skip:     w.skipDir,
		gitDirs:  w.gitWatchDirs(),
		refTrees: w.gitRefRoots(),
		anchors:  w.anchors(),
	}
}

// closeBackend closes the backend exactly once, as the loop exits.
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

// Stop cancels the loop, and with it a git recompute in flight, then waits for
// the loop to exit. It is idempotent and safe before, during, or after Start;
// before Start, it makes the next Start close its backend instead of looping.
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

// loop is the main event loop. It debounces events and calls handleBurst.
func (w *Watcher) loop(
	ctx context.Context,
) {
	// Registered first so it runs last: loopDone is closed only once every other
	// teardown defer (including the panic recovery) has run, which makes it a
	// sound "the loop goroutine is gone" signal.
	defer close(w.loopDone)
	defer safego.Recover("fs.watch.loop")
	defer w.closeBackend()

	// No initial git emit on subscribe: the snapshot already carries fresh git status, and a racing
	// duplicate could leak to another workspace's client or suppress the first real change.
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
			// swallow individual watch errors
		case <-pending.ready():
			w.handleBurst(ctx, pending.take())
		case <-w.gitTimer.Chan():
			w.runGitRecompute(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// handleBurst processes one debounce window's events, already merged per path
// by burst. The git recompute is scheduled exactly once per burst (not once per
// event) and further coalesced across bursts by scheduleGitRecompute.
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

// handleOne reports whether the event can change what a git recompute would
// answer: a ref moved or a working-tree path changed. Other git-internal churn
// (the index, lock files) cannot.
func (w *Watcher) handleOne(
	ctx context.Context,
	evt event,
) bool {
	// Before dispatching, so a dispatched directory is already being watched.
	if evt.Op&opCreate != 0 {
		w.be.created(ctx, evt.Name)
	}
	if evt.Op&opRemove != 0 {
		w.be.removed(evt.Name)
	}
	if isAnchor, ok := w.anchorEntry(evt.Name); ok {
		// The main worktree itself came or went: refs may differ and git may not read us.
		if isAnchor {
			w.diffBase = nil
		}
		return isAnchor
	}
	// Checked before shouldIgnore: a main worktree's ref events are not "ignored"
	// (they pass as file changes), a linked worktree's are, and both move the base.
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
	// The git recompute is intentionally NOT scheduled here; handleBurst
	// schedules it once for the whole burst (Fix 2).
	return true
}

// skipDir reports whether a working-tree directory is left unwatched: a .git
// dir (its refs are watched separately) or a gitignored one such as
// node_modules/, matched in-process rather than by forking `git check-ignore`.
func (w *Watcher) skipDir(
	path string,
) bool {
	return filepath.Base(path) == ".git" || w.ignore.Match(path)
}

func (w *Watcher) shouldIgnore(
	path string,
) bool {
	rel := w.relPath(path)
	// ".gitignore", ".github/" and friends are working-tree files, not git internals.
	if !isUnder(rel, ".git") {
		// A linked worktree's gitdir and common dir sit outside the worktree; their events drive the recompute
		// and are never file changes, which would ship a path escaping the workspace.
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
