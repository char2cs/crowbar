// Package watch wraps fsnotify with recursive watching and the fan-out pipeline
// defined in 05 §5 and 03 §5.
package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const debounceDuration = 100 * time.Millisecond

// Watcher watches a workspace's repo root and fans out every debounced event
// to a Dispatcher. It self-manages recursive watching for subdirectories.
type Watcher struct {
	wsID         string
	repoPath     string
	forkPointSha string
	git          GitStatusProvider
	dispatcher   Dispatcher

	fsw     *fsnotify.Watcher
	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool

	prevAdded    int
	prevDeleted  int
	prevConflict bool
	prevCommits  bool
}

// NewWatcher builds a Watcher but does not start it. Call Start to begin.
func NewWatcher(
	wsID string,
	repoPath string,
	forkPointSha string,
	git GitStatusProvider,
	dispatcher Dispatcher,
) *Watcher {
	return &Watcher{
		wsID:         wsID,
		repoPath:     repoPath,
		forkPointSha: forkPointSha,
		git:          git,
		dispatcher:   dispatcher,
		stopCh:       make(chan struct{}),
	}
}

// Start begins watching the repo root recursively. It blocks until ctx is
// cancelled or Stop is called.
func (w *Watcher) Start(
	ctx context.Context,
) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.fsw = fsw
	w.mu.Unlock()

	if err := w.addRecursive(w.repoPath); err != nil {
		fsw.Close()
		return err
	}

	go w.loop(ctx)
	return nil
}

// Stop tears down the watcher.
func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	close(w.stopCh)
	if w.fsw != nil {
		w.fsw.Close()
	}
}

func (w *Watcher) loop(
	ctx context.Context,
) {
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	var pending []fsnotify.Event

	for {
		select {
		case evt, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			pending = append(pending, evt)
			timer.Reset(debounceDuration)
		case <-w.fsw.Errors:
			// swallow individual watch errors
		case <-timer.C:
			w.handleBurst(ctx, pending)
			pending = pending[:0]
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		}
	}
}

func (w *Watcher) handleBurst(
	ctx context.Context,
	events []fsnotify.Event,
) {
	for _, evt := range events {
		w.handleOne(ctx, evt)
	}
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

	w.fanOutGit(ctx)
}

func (w *Watcher) maybeHandleGitRef(
	ctx context.Context,
	evt fsnotify.Event,
) {
	name := evt.Name
	isHead := strings.HasSuffix(name, filepath.Join(".git", "HEAD"))
	isRef := strings.Contains(name, filepath.Join(".git", "refs"))
	if !isHead && !isRef {
		return
	}
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
	w.dispatcher.OnGitStatus(ctx, w.wsID, status)

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
	checks := []string{
		filepath.Join(w.repoPath, ".git", "MERGE_HEAD"),
		filepath.Join(w.repoPath, ".git", "rebase-merge"),
		filepath.Join(w.repoPath, ".git", "rebase-apply"),
	}
	for _, path := range checks {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func (w *Watcher) addRecursive(
	root string,
) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if w.shouldIgnoreDir(path) {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
}

func (w *Watcher) shouldIgnoreDir(
	path string,
) bool {
	base := filepath.Base(path)
	if base == ".git" {
		return true
	}
	return false
}

func (w *Watcher) shouldIgnore(
	path string,
) bool {
	rel := w.relPath(path)
	if !strings.HasPrefix(rel, ".git") {
		return false
	}
	isHead := rel == filepath.Join(".git", "HEAD")
	isRefs := strings.HasPrefix(rel, filepath.Join(".git", "refs"))
	return !isHead && !isRefs
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
