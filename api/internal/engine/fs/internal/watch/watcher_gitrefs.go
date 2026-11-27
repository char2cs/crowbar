package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

func (w *Watcher) gitWatchDirs() []string {
	return []string{w.gitDir(), w.commonDir()}
}

func (w *Watcher) gitRefRoots() []string {
	return []string{
		filepath.Join(w.gitDir(), "refs"),
		filepath.Join(w.commonDir(), "refs"),
	}
}

func (w *Watcher) anchors() []string {
	common := w.commonDir()
	if filepath.Base(common) != ".git" {
		return nil
	}
	return []string{filepath.Dir(common)}
}

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

func (w *Watcher) gitDir() string {
	w.gitDirOnce.Do(func() {
		dotGit := filepath.Join(w.repoPath, ".git")
		if info, err := os.Stat(dotGit); err == nil && info.IsDir() {
			w.resolvedGitDir = dotGit
			return
		}
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
