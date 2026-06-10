// Package discover walks a directory tree looking for git repositories,
// bounded by a caller-specified depth limit (00 §5.7).
package discover

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// Repos returns the absolute paths of every directory at or shallower than
// maxDepth (measured from root) that contains a direct .git directory.
// A directory whose .git is a file (a gitdir pointer, i.e. a linked worktree
// or submodule checkout) is not a repo of its own and is skipped. Descent
// stops at any found repo so nested repos are never returned. maxDepth=1
// means only direct children of root are scanned.
func Repos(
	root string,
	maxDepth int,
) ([]string, error) {
	w := &walker{
		root:     root,
		maxDepth: maxDepth,
	}
	if err := filepath.WalkDir(root, w.visit); err != nil {
		return nil, err
	}
	return w.found, nil
}

type walker struct {
	root     string
	maxDepth int
	found    []string
}

func (w *walker) visit(
	path string,
	d fs.DirEntry,
	err error,
) error {
	if err != nil {
		return err
	}

	// Only a .git DIRECTORY marks a repo (normal checkout). A .git FILE is a
	// gitdir pointer (linked worktree / submodule): the checkout belongs to
	// another repo, so it must not be discovered as a repo of its own —
	// otherwise importing a repo that contains linked worktrees registers each
	// worktree as a separate repo and adopts every worktree once per "repo".
	// Either way the containing directory is not descended further.
	if d.Name() == ".git" {
		if d.IsDir() {
			w.found = append(w.found, filepath.Dir(path))
		}
		return filepath.SkipDir
	}

	if !d.IsDir() {
		return nil
	}

	if w.isKnownRepo(path) {
		return filepath.SkipDir
	}

	if w.depthOf(path) > w.maxDepth {
		return filepath.SkipDir
	}

	return nil
}

func (w *walker) depthOf(
	path string,
) int {
	rel, err := filepath.Rel(w.root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

func (w *walker) isKnownRepo(
	path string,
) bool {
	for _, r := range w.found {
		if strings.HasPrefix(path, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
