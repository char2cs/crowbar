package watch

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// gitWatchDirs returns the git directories watched as directories, not as the
// HEAD and packed-refs files inside them: git replaces those two by rename, which
// ends a watch on the file itself after the first change. The two roots differ for
// a linked worktree: HEAD belongs to that worktree's own gitdir, packed-refs and
// the shared ref tree to the common dir of the parent repo. For a main worktree
// they coincide and the duplicate Add collapses (fsnotify.Add is idempotent).
func (w *Watcher) gitWatchDirs() []string {
	return []string{w.gitDir(), w.commonDir()}
}

// gitRefRoots returns the ref trees whose every directory is watched: a branch
// named feat/x or a remote-tracking ref origin/main sits below refs/heads or
// refs/remotes, and fsnotify watches one directory level only.
func (w *Watcher) gitRefRoots() []string {
	return []string{
		filepath.Join(w.gitDir(), "refs"),
		filepath.Join(w.commonDir(), "refs"),
	}
}

// watchRefTree watches root and every directory below it. A path that is not a
// directory, or no longer exists, adds nothing.
func (w *Watcher) watchRefTree(
	root string,
) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = w.fsw.Add(path)
		}
		return nil
	})
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
