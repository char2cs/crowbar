// Package watch wraps fsnotify with recursive watching and the fan-out pipeline
// defined in 05 §5 and 03 §5.
package watch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

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
// exactly one Start. Re-Starting would leak the first fsnotify FD and its loop
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

	fsw          *fsnotify.Watcher
	mu           sync.Mutex
	stopCh       chan struct{}
	started      bool
	stopped      bool
	fswCloseOnce sync.Once

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
// cancelled or Stop is called. Every *fsnotify.Watcher Start creates is closed
// exactly once regardless of cancel/stop ordering: if ctx is already cancelled
// or Stop already ran before registration, the freshly created watcher is closed
// immediately and Start returns without leaking an inotify FD or a loop goroutine.
//
// A Watcher is single-use: calling Start a second time (after a prior Start, or
// after Stop) returns ErrAlreadyStarted and closes the freshly created watcher
// in place rather than overwriting w.fsw and leaking the first FD and loop
// goroutine.
func (w *Watcher) Start(
	ctx context.Context,
) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watch: start: new fsnotify watcher: %w", err)
	}

	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		_ = fsw.Close()
		return ErrAlreadyStarted
	}
	if w.stopped {
		w.mu.Unlock()
		_ = fsw.Close()
		return context.Canceled
	}
	w.started = true
	w.fsw = fsw
	w.mu.Unlock()

	if err := w.addRecursive(w.repoPath); err != nil {
		w.closeFSW()
		return fmt.Errorf("watch: start: add recursive %s: %w", w.repoPath, err)
	}

	// Explicitly watch the git directories and the ref trees so branch switches
	// and remote fetches are detected: addRecursive skips .git entirely (it would
	// flood inotify with objects/logs), so these are the only git-internal paths
	// watched (05 §5). They are resolved, never joined onto repoPath/.git — in a
	// Crowbar-managed workspace (always a LINKED worktree) that is a gitlink FILE,
	// so every such join lands on a path that does not exist and every Add fails
	// silently, leaving a ref-only change (a bare fetch, which touches no
	// working-tree file) with nothing to trigger the recompute.
	for _, dir := range w.gitWatchDirs() {
		// ignore errors — a path may legitimately be absent
		_ = w.fsw.Add(dir)
	}
	for _, root := range w.gitRefRoots() {
		w.watchRefTree(root)
	}

	go w.loop(ctx)
	return nil
}

// closeFSW closes the fsnotify watcher exactly once. It is safe to call from
// Start's early-return paths, from loop on exit, and from Stop concurrently.
func (w *Watcher) closeFSW() {
	w.fswCloseOnce.Do(func() {
		w.mu.Lock()
		fsw := w.fsw
		w.mu.Unlock()
		if fsw != nil {
			_ = fsw.Close()
		}
	})
}

// Stop tears down the watcher. It is idempotent and safe to call before, during,
// or after Start. Calling Stop before Start makes the next Start observe stopped
// and close its watcher immediately rather than entering the loop.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	close(w.stopCh)
	w.mu.Unlock()

	w.closeFSW()
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
	defer w.closeFSW()

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
	var pending []fsnotify.Event

	for {
		select {
		case evt, ok := <-w.fsw.Events:
			if !w.processEvent(evt, ok, &pending, timer) {
				return
			}
		case <-w.fsw.Errors:
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
	evt fsnotify.Event,
	ok bool,
	pending *[]fsnotify.Event,
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
	events []fsnotify.Event,
) {
	merged := make(map[string]fsnotify.Op, len(events))
	order := make([]string, 0, len(events))
	for _, evt := range events {
		if _, seen := merged[evt.Name]; !seen {
			order = append(order, evt.Name)
		}
		merged[evt.Name] |= evt.Op
	}
	relevant := false
	for _, name := range order {
		if w.handleOne(ctx, fsnotify.Event{Name: name, Op: merged[name]}) {
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
	evt fsnotify.Event,
) bool {
	// Checked before shouldIgnore: a main worktree's ref events are not "ignored"
	// (they pass as file changes), a linked worktree's are, and both move the base.
	isRef := w.isGitRefPath(evt.Name)
	if isRef {
		w.diffBase = nil
		if evt.Op&fsnotify.Create != 0 {
			w.watchRefTree(evt.Name)
		}
	}
	if w.shouldIgnore(evt.Name) {
		w.maybeHandleGitRef(ctx, evt)
		return isRef
	}

	rel := w.relPath(evt.Name)
	changeEvt := domain.FileChangeEvent{
		Type: classifyChange(evt.Op),
		Path: rel,
		WsID: w.wsID,
	}
	w.dispatcher.OnFileChange(ctx, changeEvt)

	if evt.Op&fsnotify.Create != 0 {
		_ = w.addRecursive(evt.Name)
	}
	if evt.Op&fsnotify.Remove != 0 {
		_ = w.fsw.Remove(evt.Name)
	}
	// The git recompute is intentionally NOT scheduled here; handleBurst
	// schedules it once for the whole burst (Fix 2).
	return true
}

// walkFn is the filepath.WalkDir callback used by addRecursive. WalkDir reads
// entry types from the directory listing, so files are never lstat'd — only
// directories are visited for real.
func (w *Watcher) walkFn(
	path string,
	d fs.DirEntry,
	err error,
) error {
	if err != nil {
		return nil
	}
	if !d.IsDir() {
		return nil
	}
	if w.shouldIgnoreDir(path) {
		return filepath.SkipDir
	}
	// Fix 3: skip gitignored directories to avoid inotify exhaustion on large
	// projects with node_modules/, dist/, etc. Matched in-process (ignore.Matcher)
	// rather than forking `git check-ignore` once per directory (~458 forks per
	// Start on a real repo).
	if w.ignore.Match(path) {
		return filepath.SkipDir
	}
	// WalkDir visits a directory before reading it, so an unreadable one reaches
	// Add; skip it like the unreadable entries Walk never offered.
	addErr := w.fsw.Add(path)
	if errors.Is(addErr, fs.ErrPermission) {
		return filepath.SkipDir
	}
	return addErr
}

// addRecursive watches root and all non-ignored subdirectories.
func (w *Watcher) addRecursive(
	root string,
) error {
	return filepath.WalkDir(root, w.walkFn)
}

func (w *Watcher) shouldIgnoreDir(
	path string,
) bool {
	base := filepath.Base(path)
	return base == ".git"
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
	op fsnotify.Op,
) domain.FileChangeType {
	if op&fsnotify.Create != 0 {
		return domain.FileChangeCreated
	}
	if op&fsnotify.Remove != 0 {
		return domain.FileChangeDeleted
	}
	if op&fsnotify.Rename != 0 {
		return domain.FileChangeRenamed
	}
	return domain.FileChangeModified
}
