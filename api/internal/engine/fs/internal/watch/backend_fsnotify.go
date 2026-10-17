//go:build !darwin

package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type (
	event = fsnotify.Event
	op    = fsnotify.Op
)

const (
	opCreate = fsnotify.Create
	opWrite  = fsnotify.Write
	opRemove = fsnotify.Remove
	opRename = fsnotify.Rename
	opChmod  = fsnotify.Chmod
)

// fsnotifyBackend watches directory by directory (inotify is not recursive), so
// it walks the spec on start and adds each directory that appears later.
type fsnotifyBackend struct {
	fsw  *fsnotify.Watcher
	spec watchSpec
}

func newBackend(
	spec watchSpec,
) (backend, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("new fsnotify watcher: %w", err)
	}
	b := &fsnotifyBackend{fsw: fsw, spec: spec}
	if err := b.addRecursive(spec.tree); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("add recursive %s: %w", spec.tree, err)
	}
	// HEAD and packed-refs are replaced by rename, which ends a watch on the file
	// itself, so their directories are watched instead. A path may be absent.
	for _, dir := range spec.gitDirs {
		_ = fsw.Add(dir)
	}
	for _, root := range spec.refTrees {
		b.watchRefTree(root)
	}
	return b, nil
}

func (b *fsnotifyBackend) events() <-chan event {
	return b.fsw.Events
}

func (b *fsnotifyBackend) errors() <-chan error {
	return b.fsw.Errors
}

func (b *fsnotifyBackend) created(
	path string,
) {
	for _, root := range b.spec.refTrees {
		if isUnder(path, root) {
			b.watchRefTree(path)
			return
		}
	}
	for _, dir := range b.spec.gitDirs {
		if isUnder(path, dir) {
			return
		}
	}
	if isUnder(path, b.spec.tree) {
		_ = b.addRecursive(path)
	}
}

func (b *fsnotifyBackend) removed(
	path string,
) {
	_ = b.fsw.Remove(path)
}

func (b *fsnotifyBackend) close() error {
	return b.fsw.Close()
}

// addRecursive watches root and every subdirectory spec.skip keeps. WalkDir
// reads entry types from the listing, so files are never lstat'd.
func (b *fsnotifyBackend) addRecursive(
	root string,
) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a directory that vanished or cannot be read is simply not watched
		}
		if !d.IsDir() {
			return nil
		}
		if b.spec.skip(path) {
			return filepath.SkipDir
		}
		// WalkDir visits a directory before reading it, so an unreadable one
		// reaches Add; skip it like the unreadable entries Walk never offered.
		addErr := b.fsw.Add(path)
		if errors.Is(addErr, fs.ErrPermission) {
			return filepath.SkipDir
		}
		return addErr
	})
}

// watchRefTree watches root and every directory below it: a branch named
// feat/x sits below refs/heads. A path that is not a directory adds nothing.
func (b *fsnotifyBackend) watchRefTree(
	root string,
) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = b.fsw.Add(path)
		}
		return nil
	})
}
