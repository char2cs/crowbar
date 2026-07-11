// Package watch wraps fsnotify with recursive watching and the fan-out pipeline
// defined in 05 §5 and 03 §5.
package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// gitRecomputeDebounce is the trailing debounce applied to git recomputes
// (fanOutGit = ComputeStatus + ComputeWorkingTreeSummary, two git invocations
// per run). Workspaces sharing one .git (linked worktrees) all observe each
// other's ref events, so without coalescing a single ref update makes every
// workspace's watcher shell out to git at once — measured as ~6Hz sustained
// churn. Bursts collapse into a single recompute that always runs once the
// watcher has been quiet for this long; the trailing recompute is never
// dropped, only delayed.
const gitRecomputeDebounce = 250 * time.Millisecond

// ErrAlreadyStarted is returned by Start when the watcher has already been
// started (or stopped). A Watcher's lifecycle is single-use: the caller owns
// exactly one Start. Re-Starting would leak the first fsnotify FD and its loop
// goroutine, so it is rejected instead.
var ErrAlreadyStarted = errors.New("watch: already started")

// Watcher watches a workspace's repo root and fans out every debounced event
// to a Dispatcher. It self-manages recursive watching for subdirectories.
type Watcher struct {
	wsID         string
	repoPath     string
	forkPointSha string
	git          GitStatusProvider
	dispatcher   Dispatcher
	ignore       ignore.Matcher

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

	// gitTimer is the trailing-debounce timer for git recomputes. It is
	// created stopped in NewWatcher, armed by scheduleGitRecompute, and
	// drained exclusively by loop's <-gitTimer.C case. gitPending tracks
	// whether a recompute is scheduled but not yet run (guarded by mu).
	gitTimer   *time.Timer
	gitPending bool
}

// gitStatusEqual reports whether two statuses carry the same broadcastable
// state (branch, ahead/behind, and the exact file list in order).
func gitStatusEqual(
	a gitdomain.GitStatus,
	b gitdomain.GitStatus,
) bool {
	if a.Branch != b.Branch || a.Ahead != b.Ahead || a.Behind != b.Behind {
		return false
	}
	if len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		if a.Files[i] != b.Files[i] {
			return false
		}
	}
	return true
}

// NewWatcher builds a Watcher but does not start it. Call Start to begin.
func NewWatcher(
	wsID string,
	repoPath string,
	forkPointSha string,
	git GitStatusProvider,
	dispatcher Dispatcher,
) *Watcher {
	gitTimer := time.NewTimer(gitRecomputeDebounce)
	if !gitTimer.Stop() {
		<-gitTimer.C
	}
	return &Watcher{
		wsID:         wsID,
		repoPath:     repoPath,
		forkPointSha: forkPointSha,
		git:          git,
		dispatcher:   dispatcher,
		ignore:       ignore.NewMatcher(repoPath),
		stopCh:       make(chan struct{}),
		gitTimer:     gitTimer,
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

	// Explicitly watch HEAD and the ref trees so branch switches and remote fetches
	// are detected: addRecursive skips .git entirely (it would flood inotify with
	// objects/logs), so these are the only git-internal paths watched (05 §5). They
	// are resolved, never joined onto repoPath/.git — in a Crowbar-managed workspace
	// (always a LINKED worktree) that is a gitlink FILE, so every such join lands on
	// a path that does not exist and every Add fails silently, leaving a ref-only
	// change (a bare fetch, which touches no working-tree file) with nothing to
	// trigger the recompute.
	for _, p := range w.gitRefWatchPaths() {
		// ignore errors — paths may legitimately be absent (no packed-refs until the
		// first gc, no refs/remotes without a remote)
		_ = w.fsw.Add(p)
	}

	go w.loop(ctx)
	return nil
}

// gitRefWatchPaths returns the git-internal paths whose changes mean "the refs
// moved". The two roots differ for a linked worktree: HEAD (and any worktree-local
// ref) belongs to that worktree's own gitdir, while the shared ref tree and
// packed-refs live in the common dir of the parent repo. For a main worktree the two
// roots coincide and the duplicate Adds collapse (fsnotify.Add is idempotent).
func (w *Watcher) gitRefWatchPaths() []string {
	gitDir := w.gitDir()
	commonDir := w.commonDir()
	return []string{
		filepath.Join(gitDir, "HEAD"),
		filepath.Join(gitDir, "refs"),
		filepath.Join(commonDir, "refs"),
		filepath.Join(commonDir, "refs", "heads"),
		filepath.Join(commonDir, "refs", "remotes"),
		filepath.Join(commonDir, "packed-refs"),
	}
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
		case <-w.gitTimer.C:
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
	for _, name := range order {
		w.handleOne(ctx, fsnotify.Event{Name: name, Op: merged[name]})
	}
	w.scheduleGitRecompute()
}

func (w *Watcher) handleOne(
	ctx context.Context,
	evt fsnotify.Event,
) {
	if w.shouldIgnore(evt.Name) {
		w.maybeHandleGitRef(ctx, evt)
		return
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
}

func (w *Watcher) maybeHandleGitRef(
	_ context.Context,
	evt fsnotify.Event,
) {
	if !w.isGitRefPath(evt.Name) {
		return
	}
	w.scheduleGitRecompute()
}

// isGitRefPath reports whether path is one of the ref paths Start watches, matched
// against the RESOLVED roots rather than by ".git/..." substring: a linked
// worktree's gitdir contains no ".git" path element at all
// (<repo>/.git/worktrees/<name>/HEAD does, but its common ref tree is
// <repo>/.git/refs/...), so substring matching answers for the wrong worktree or not
// at all.
func (w *Watcher) isGitRefPath(
	path string,
) bool {
	for _, root := range []string{w.gitDir(), w.commonDir()} {
		if path == filepath.Join(root, "HEAD") || path == filepath.Join(root, "packed-refs") {
			return true
		}
		if isUnder(path, filepath.Join(root, "refs")) {
			return true
		}
	}
	return false
}

// isGitInternal reports whether path belongs to the repository's own machinery (the
// worktree's gitdir or the shared common dir) rather than to the working tree.
func (w *Watcher) isGitInternal(
	path string,
) bool {
	return isUnder(path, w.gitDir()) || isUnder(path, w.commonDir())
}

func isUnder(
	path string,
	root string,
) bool {
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// scheduleGitRecompute arms (or re-arms) the trailing git-recompute debounce.
// Back-to-back calls within gitRecomputeDebounce collapse into one fanOutGit
// run, executed by loop once the quiet period elapses. It is called only from
// the loop goroutine (handleBurst / maybeHandleGitRef), so resetting the timer
// never races with loop's drain of gitTimer.C.
func (w *Watcher) scheduleGitRecompute() {
	w.mu.Lock()
	w.gitPending = true
	w.mu.Unlock()
	w.gitTimer.Reset(gitRecomputeDebounce)
}

// runGitRecompute clears the pending flag and performs the coalesced git
// recompute. It runs in the loop goroutine when gitTimer fires.
func (w *Watcher) runGitRecompute(
	ctx context.Context,
) {
	w.mu.Lock()
	w.gitPending = false
	w.mu.Unlock()
	w.fanOutGit(ctx)
}

func (w *Watcher) fanOutGit(
	ctx context.Context,
) {
	if w.isRewriteInProgress() {
		return
	}

	status, err := w.git.ComputeStatus(ctx, w.repoPath)
	if err != nil {
		return
	}
	// Workspaces sharing a .git (linked worktrees) all see each other's ref
	// events; without this guard every such event re-broadcasts an unchanged
	// status to every subscriber (observed as a ~6Hz identical-frame storm).
	// prevStatus starts unset, so the FIRST recompute always broadcasts; since the
	// watcher does no initial recompute on subscribe (see loop — the snapshot
	// already delivers fresh status), that first broadcast is the first real
	// file-change, and subsequent identical frames are deduped here.
	if !w.prevStatusSet || !gitStatusEqual(w.prevStatus, status) {
		w.prevStatus = status
		w.prevStatusSet = true
		w.dispatcher.OnGitStatus(ctx, w.wsID, status)
	}

	added, deleted, hasConflicts, hasCommits, err := w.git.ComputeWorkingTreeSummary(
		ctx,
		w.repoPath,
		w.forkPointSha,
	)
	if err != nil {
		return
	}

	if added == w.prevAdded && deleted == w.prevDeleted &&
		hasConflicts == w.prevConflict && hasCommits == w.prevCommits {
		return
	}

	w.prevAdded = added
	w.prevDeleted = deleted
	w.prevConflict = hasConflicts
	w.prevCommits = hasCommits

	w.dispatcher.OnSyncWorkingTreeState(ctx, SyncInput{
		WsID:         w.wsID,
		Added:        added,
		Deleted:      deleted,
		HasConflicts: hasConflicts,
		HasCommits:   hasCommits,
	})
}

func (w *Watcher) isRewriteInProgress() bool {
	gitDir := w.gitDir()
	checks := []string{
		filepath.Join(gitDir, "MERGE_HEAD"),
		filepath.Join(gitDir, "rebase-merge"),
		filepath.Join(gitDir, "rebase-apply"),
	}
	for _, path := range checks {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// gitDir resolves and caches the real git directory for repoPath. The main
// worktree's repoPath/.git is a directory; a child (linked) worktree's .git is a
// FILE ("gitdir: <path>") pointing at the per-worktree dir under the common dir —
// which is where that worktree's MERGE_HEAD / rebase-merge / rebase-apply live.
// Statting repoPath/.git/<marker> directly returns ENOTDIR for a linked worktree,
// so the rewrite guard would always read "false" mid-rebase/merge and broadcast
// transient, wrong status frames during exactly the rewrite it should skip.
func (w *Watcher) gitDir() string {
	w.gitDirOnce.Do(func() {
		dotGit := filepath.Join(w.repoPath, ".git")
		if info, err := os.Stat(dotGit); err == nil && info.IsDir() {
			w.resolvedGitDir = dotGit
			return
		}
		// .git is a gitlink file ("gitdir: <path>"). Fall back to dotGit on any
		// parse failure (worst case: the guard behaves as before for this worktree).
		w.resolvedGitDir = dotGit
		data, err := os.ReadFile(dotGit) //nolint:gosec // G304: dotGit is repoPath/.git, a fixed path derived from the watched repo
		if err != nil {
			return
		}
		line := strings.TrimSpace(string(data))
		const prefix = "gitdir:"
		if !strings.HasPrefix(line, prefix) {
			return
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if !filepath.IsAbs(p) {
			p = filepath.Join(w.repoPath, p)
		}
		w.resolvedGitDir = p
	})
	return w.resolvedGitDir
}

// commonDir resolves and caches the git COMMON dir: the directory holding the state
// every worktree of a repository shares — refs/heads, refs/remotes, packed-refs,
// objects. For a main worktree it IS the git dir; for a linked worktree the git dir
// holds only that worktree's private state (HEAD, index) plus a "commondir" file
// naming the shared one. Watching refs under the git dir alone would therefore watch
// a linked worktree's empty private refs/ and miss every branch and remote-tracking
// update. Falls back to the git dir when commondir is absent or unreadable, which is
// exactly the main-worktree layout.
func (w *Watcher) commonDir() string {
	w.commonDirOnce.Do(func() {
		gitDir := w.gitDir()
		w.resolvedCommonDir = gitDir

		data, err := os.ReadFile(filepath.Join(gitDir, "commondir")) //nolint:gosec // G304: a fixed filename under the resolved git dir of the watched repo
		if err != nil {
			return
		}
		p := strings.TrimSpace(string(data))
		if p == "" {
			return
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(gitDir, p)
		}
		w.resolvedCommonDir = filepath.Clean(p)
	})
	return w.resolvedCommonDir
}

// walkFn is the filepath.Walk callback used by addRecursive.
// Fix 7: extracted from an anonymous closure to a named method.
func (w *Watcher) walkFn(
	path string,
	info os.FileInfo,
	err error,
) error {
	if err != nil {
		return nil
	}
	if !info.IsDir() {
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
	return w.fsw.Add(path)
}

// addRecursive watches root and all non-ignored subdirectories.
func (w *Watcher) addRecursive(
	root string,
) error {
	return filepath.Walk(root, w.walkFn)
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
