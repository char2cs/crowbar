// Package watch watches a workspace recursively (inotify on Linux, FSEvents on
// darwin) and runs the fan-out pipeline defined in 05 §5 and 03 §5.
package watch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch/ignore"
)

const debounceDuration = 100 * time.Millisecond

// A git recompute (status + diff stat, two git processes) waits for changes to
// stop for gitRecomputeQuiet, so an agent writing files in bursts costs one
// recompute per burst rather than one per pause. gitRecomputeMaxWait bounds the
// delay for a burst that never ends. Workspaces sharing one .git (linked
// worktrees) all observe each other's ref events, so the same wait also keeps a
// single ref update from making every workspace shell out at once.
const (
	gitRecomputeQuiet   = 2 * time.Second
	gitRecomputeMaxWait = 10 * time.Second
)

// ErrAlreadyStarted is returned by Start when the watcher has already been
// started (or stopped). A Watcher's lifecycle is single-use: the caller owns
// exactly one Start. Re-Starting would leak the first backend and its loop
// goroutine, so it is rejected instead.
var ErrAlreadyStarted = errors.New("watch: already started")

// debounceTimer is the timer behind the trailing git-recompute debounce. It is
// an interface so that a test can substitute a timer it fires by hand: the
// coalescing guarantee (N bursts collapse into exactly one recompute) is then
// proven by firing once and counting recomputes, rather than by racing a wall
// clock and hoping the bursts landed inside the window. Production always uses
// realTimer.
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
	stopCh      chan struct{}
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

	// diffBase is the resolved base of the last recompute, kept until a ref
	// event says refs moved: it is a function of refs alone, so a working-tree
	// burst never changes it. Only the loop goroutine touches it.
	diffBase *gitdomain.DiffBase

	// gitTimer is the trailing-debounce timer for git recomputes. It is
	// created stopped in NewWatcher, armed by scheduleGitRecompute, and
	// drained exclusively by loop's <-gitTimer.Chan() case. gitPending tracks
	// whether a recompute is scheduled but not yet run (guarded by mu).
	gitTimer   debounceTimer
	gitPending bool
	// gitPendingSince is when the oldest unrun change scheduled the recompute;
	// the max wait counts from it (guarded by mu).
	gitPendingSince time.Time

	// now is the clock the max wait reads; a test injects its own.
	now func() time.Time

	// loopDone is closed when the loop goroutine returns. It is the real signal
	// that the watcher has finished tearing down (used by tests to block on the
	// loop's exit instead of polling the process's goroutine count).
	loopDone chan struct{}

	// onGitRecompute, when non-nil, is invoked by the loop goroutine after every
	// completed git recompute (including one that fanOutGit short-circuits, e.g.
	// mid-rebase). It is nil in production and set only by tests — before Start,
	// so the `go w.loop` that follows establishes the happens-before edge — so a
	// test can block on "the debounce flushed" instead of sleeping past it.
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
		stopCh:     make(chan struct{}),
		loopDone:   make(chan struct{}),
		gitTimer:   newStoppedTimer(gitRecomputeQuiet),
		now:        time.Now,
	}
}

// Start begins watching the repo root recursively. It blocks until ctx is
// cancelled or Stop is called. Every backend Start creates is closed exactly
// once regardless of cancel/stop ordering: if ctx is already cancelled or Stop
// already ran before registration, the freshly created backend is closed
// immediately and Start returns without leaking a descriptor or a goroutine.
//
// A Watcher is single-use: calling Start a second time (after a prior Start, or
// after Stop) returns ErrAlreadyStarted and closes the freshly created backend
// in place rather than overwriting w.be and leaking the first one.
func (w *Watcher) Start(
	ctx context.Context,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	be, err := newBackend(w.spec())
	if err != nil {
		return fmt.Errorf("watch: start: %w", err)
	}

	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		_ = be.close()
		return ErrAlreadyStarted
	}
	if w.stopped {
		w.mu.Unlock()
		_ = be.close()
		return context.Canceled
	}
	w.started = true
	w.be = be
	w.mu.Unlock()

	go w.loop(ctx)
	return nil
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
	}
}

// closeBackend closes the backend exactly once. It is safe to call from loop
// on exit and from Stop concurrently.
func (w *Watcher) closeBackend() {
	w.beCloseOnce.Do(func() {
		w.mu.Lock()
		be := w.be
		w.mu.Unlock()
		if be != nil {
			_ = be.close()
		}
	})
}

// Stop tears down the watcher. It is idempotent and safe to call before, during,
// or after Start. Calling Stop before Start makes the next Start observe stopped
// and close its backend immediately rather than entering the loop.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	close(w.stopCh)
	w.mu.Unlock()

	w.closeBackend()
}

// loop is the main event loop. It debounces events and calls handleBurst.
// The select is kept at exactly one level of nesting via processEvent.
func (w *Watcher) loop(
	ctx context.Context,
) {
	// Registered first so it runs last: loopDone is closed only once every other
	// teardown defer (including the panic recovery) has run, which makes it a
	// sound "the loop goroutine is gone" signal.
	defer close(w.loopDone)
	defer safego.Recover("fs.watch.loop")
	defer w.closeBackend()

	// No initial git emit on subscribe: the snapshot-on-subscribe already delivers
	// fresh git status (gitSnapshot reads real `git status`, not the read model),
	// so an initial broadcast would be a redundant duplicate of the snapshot — and,
	// racing ahead of or behind the snapshot, it would either leak a stray frame to
	// a wsId-scoped client (cross-workspace isolation) or suppress the first real
	// change. Live git frames are driven purely by real file changes; the
	// read-model summary badge is kept fresh by lazy reconcile-on-open and by
	// file-change recomputes (spec §3.8).
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	var pending []event
	events, errs := w.be.events(), w.be.errors()

	for {
		select {
		case evt, ok := <-events:
			if !w.processEvent(evt, ok, &pending, timer) {
				return
			}
		case <-errs:
			// swallow individual watch errors
		case <-timer.C:
			w.handleBurst(ctx, pending)
			pending = pending[:0]
		case <-w.gitTimer.Chan():
			w.runGitRecompute(ctx)
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		}
	}
}

// processEvent appends an event to pending and resets the debounce timer.
// Returns false when the events channel is closed (loop should exit).
func (w *Watcher) processEvent(
	evt event,
	ok bool,
	pending *[]event,
	debounce *time.Timer,
) bool {
	if !ok {
		return false
	}
	*pending = append(*pending, evt)
	debounce.Reset(debounceDuration)
	return true
}

// handleBurst processes all events from a single debounce window.
// Events for the same path are merged (OR of ops) so that CREATE+WRITE
// on Linux inotify is reported as a single "created" event, not "modified".
// The git recompute is scheduled exactly once per burst (not once per event)
// and further coalesced across bursts by scheduleGitRecompute.
func (w *Watcher) handleBurst(
	ctx context.Context,
	events []event,
) {
	merged := make(map[string]op, len(events))
	order := make([]string, 0, len(events))
	for _, evt := range events {
		if _, seen := merged[evt.Name]; !seen {
			order = append(order, evt.Name)
		}
		merged[evt.Name] |= evt.Op
	}
	relevant := false
	for _, name := range order {
		if w.handleOne(ctx, event{Name: name, Op: merged[name]}) {
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
		w.be.created(evt.Name)
	}
	if evt.Op&opRemove != 0 {
		w.be.removed(evt.Name)
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
		Type: classifyChange(evt.Op),
		Path: w.relPath(evt.Name),
		WsID: w.wsID,
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
	if !strings.HasPrefix(rel, ".git") {
		// A linked worktree's gitdir, and the common dir holding the shared ref tree,
		// sit OUTSIDE the worktree, so no relative-path test can recognise them. Their
		// events are git internals: they drive the recompute (maybeHandleGitRef) and are
		// never file changes — reporting one as a file change would ship a path that
		// escapes the workspace ("../../../repo/.git/refs/heads/main").
		return w.isGitInternal(path)
	}
	isHead := rel == filepath.Join(".git", "HEAD")
	isRefs := strings.HasPrefix(rel, filepath.Join(".git", "refs"))
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
