// Package discover walks a directory tree looking for git repositories,
// bounded by a caller-specified depth limit.
package discover

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// Repos returns every directory within maxDepth of root (1 = direct children) that
// holds a .git directory. A .git file (linked worktree, submodule) is not a repo of
// its own, and descent stops at a found repo so nested repos are never returned.
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

	// Only a .git directory marks a repo: a .git file is a linked worktree or submodule
	// belonging to another repo, which would otherwise be imported once per worktree.
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
