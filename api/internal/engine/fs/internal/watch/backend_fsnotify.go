//go:build !darwin

package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"syscall"

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
		return nil, errors.Join(fmt.Errorf("add recursive %s: %w", spec.tree, err), fsw.Close())
	}
	// HEAD and packed-refs are replaced by rename, which ends a watch on the file
	// itself, so their directories are watched instead.
	for _, dir := range spec.gitDirs {
		b.watchBestEffort(dir)
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
		if err := b.addRecursive(path); err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fsnotify.ErrClosed) {
			slog.Warn("watch: follow new directory", "path", path, "err", err)
		}
	}
}

func (b *fsnotifyBackend) removed(
	path string,
) {
	// The kernel drops a deleted directory's watch itself (EINVAL on the explicit
	// removal), and files were never watched, so a watch already gone is not a failure.
	err := b.fsw.Remove(path)
	if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) && !errors.Is(err, syscall.EINVAL) {
		slog.Warn("watch: drop watch", "path", path, "err", err)
	}
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
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			return b.add(path)
		}
		return nil
	})
	if err != nil {
		slog.Warn("watch: ref tree not fully watched", "root", root, "err", err)
	}
}

// watchBestEffort watches dir, logging the failures that are not a path being
// absent or unreadable.
func (b *fsnotifyBackend) watchBestEffort(
	dir string,
) {
	if err := b.add(dir); err != nil {
		slog.Warn("watch: directory not watched", "dir", dir, "err", err)
	}
}

// add watches dir. A dir that is absent or unreadable is not an error (git dirs and ref
// trees may not exist yet), nor is a watcher that Stop has already closed.
func (b *fsnotifyBackend) add(
	dir string,
) error {
	err := b.fsw.Add(dir)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, fsnotify.ErrClosed) {
		return nil
	}
	return err
}
