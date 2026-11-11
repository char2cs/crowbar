package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// gitWatchDirs returns git dirs watched as directories: git replaces HEAD and packed-refs by rename,
// ending a file watch. A linked worktree's HEAD is in its gitdir, shared refs in the common dir.
func (w *Watcher) gitWatchDirs() []string {
	return []string{w.gitDir(), w.commonDir()}
}

// gitRefRoots returns the ref trees whose every directory is watched: a branch
// named feat/x or a remote-tracking ref origin/main sits below refs/heads or
// refs/remotes, and inotify watches one directory level only.
func (w *Watcher) gitRefRoots() []string {
	return []string{
		filepath.Join(w.gitDir(), "refs"),
		filepath.Join(w.commonDir(), "refs"),
	}
}

// anchors names the main worktree's directory: moving or deleting it takes the
// common dir with it, and inotify sends nothing to watches below a moved directory.
// A bare common dir has no worktree directory to anchor on.
func (w *Watcher) anchors() []string {
	common := w.commonDir()
	if filepath.Base(common) != ".git" {
		return nil
	}
	return []string{filepath.Dir(common)}
}

// anchorEntry reports whether path is an entry of an anchor's parent directory, and if
// so whether it is the anchor itself; its siblings are unrelated and dropped.
func (w *Watcher) anchorEntry(
	path string,
) (isAnchor, ok bool) {
	for _, anchor := range w.anchors() {
		if path == anchor {
			return true, true
		}
		if filepath.Dir(path) == filepath.Dir(anchor) && !isUnder(path, w.repoPath) {
			return false, true
		}
	}
	return false, false
}

func (w *Watcher) maybeHandleGitRef(
	_ context.Context,
	evt event,
) {
	if !w.isGitRefPath(evt.Name) {
		return
	}
	w.scheduleGitRecompute()
}

// isGitRefPath matches against the resolved roots, not a ".git/" substring: a linked worktree's ref
// tree lives under the parent repo, so substring matching answers for the wrong worktree.
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

// gitDir resolves and caches the real git dir: a linked worktree's .git is a "gitdir:" FILE, so
// statting repoPath/.git/<marker> would hit ENOTDIR and mid-rebase guards would always read false.
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

// commonDir resolves and caches the dir all worktrees share (refs, packed-refs, objects); a linked
// worktree's own gitdir has empty refs/, so watching it would miss every branch update. Falls back to
// the git dir when commondir is absent, which is the main-worktree layout.
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
