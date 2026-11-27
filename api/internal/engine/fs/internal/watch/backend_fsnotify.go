//go:build !darwin

package watch

import (
	"context"
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

type fsnotifyBackend struct {
	fsw  *fsnotify.Watcher
	spec watchSpec
}

func newBackend(
	ctx context.Context,
	spec watchSpec,
) (backend, error) {
	if spec.file {
		return newFileBackend(spec.tree)
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("new fsnotify watcher: %w", err)
	}
	b := &fsnotifyBackend{fsw: fsw, spec: spec}
	if err := b.addRecursive(ctx, spec.tree); err != nil {
		return nil, errors.Join(fmt.Errorf("add recursive %s: %w", spec.tree, err), fsw.Close())
	}
	for _, dir := range spec.gitDirs {
		b.watchBestEffort(dir)
	}
	for _, root := range spec.refTrees {
		b.watchRefTree(root)
	}
	for _, anchor := range spec.anchors {
		b.watchBestEffort(filepath.Dir(anchor))
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
	ctx context.Context,
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
		err := b.addRecursive(ctx, path)
		if err != nil && ctx.Err() == nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fsnotify.ErrClosed) {
			slog.WarnContext(ctx, "watch: follow new directory", "path", path, "err", err)
		}
	}
}

func (b *fsnotifyBackend) removed(
	path string,
) {
	dropWatch(b.fsw, path)
}

func dropWatch(
	fsw *fsnotify.Watcher,
	path string,
) {
	err := fsw.Remove(path)
	if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) && !errors.Is(err, syscall.EINVAL) {
		slog.Warn("watch: drop watch", "path", path, "err", err)
	}
}

func (b *fsnotifyBackend) close() error {
	return b.fsw.Close()
}

func (b *fsnotifyBackend) addRecursive(
	ctx context.Context,
	root string,
) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a directory that vanished or cannot be read is simply not watched
		}
		if !d.IsDir() {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if b.spec.skip(path) {
			return filepath.SkipDir
		}
		addErr := b.fsw.Add(path)
		if errors.Is(addErr, fs.ErrPermission) {
			return filepath.SkipDir
		}
		return addErr
	})
}

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

func (b *fsnotifyBackend) watchBestEffort(
	dir string,
) {
	if err := b.add(dir); err != nil {
		slog.Warn("watch: directory not watched", "dir", dir, "err", err)
	}
}

func (b *fsnotifyBackend) add(
	dir string,
) error {
	err := b.fsw.Add(dir)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, fsnotify.ErrClosed) {
		return nil
	}
	return err
}
