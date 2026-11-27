//go:build !darwin

package watch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

type fileBackend struct {
	fsw     *fsnotify.Watcher
	path    string
	parent  string
	watched string
	out     chan event
	done    chan struct{}
	exited  chan struct{}
	once    sync.Once
}

func newFileBackend(
	path string,
) (backend, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, watchLimit(fmt.Errorf("new fsnotify watcher: %w", err))
	}
	b := &fileBackend{
		fsw:    fsw,
		path:   path,
		parent: filepath.Dir(path),
		out:    make(chan event),
		done:   make(chan struct{}),
		exited: make(chan struct{}),
	}
	if err := b.arm(); err != nil {
		return nil, errors.Join(watchLimit(err), fsw.Close())
	}
	go b.relay()
	return b, nil
}

func (b *fileBackend) events() <-chan event {
	return b.out
}

func (b *fileBackend) errors() <-chan error {
	return b.fsw.Errors
}

func (b *fileBackend) created(context.Context, string) {}

func (b *fileBackend) removed(string) {}

func (b *fileBackend) close() error {
	var err error
	b.once.Do(func() {
		close(b.done)
		err = b.fsw.Close()
		<-b.exited
	})
	return err
}

func (b *fileBackend) relay() {
	defer close(b.exited)
	defer close(b.out)
	for {
		select {
		case evt, ok := <-b.fsw.Events:
			if !ok || !b.forward(evt) {
				return
			}
		case <-b.done:
			return
		}
	}
}

func (b *fileBackend) forward(
	evt fsnotify.Event,
) bool {
	switch {
	case evt.Name == b.path:
		return b.send(evt.Op)
	case evt.Name == b.watched && evt.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
		if b.watched == b.parent && !b.send(opRemove) {
			return false
		}
		return b.rearm()
	case b.watched != b.parent && evt.Op&fsnotify.Create != 0 && isUnder(b.parent, evt.Name):
		return b.rearm()
	}
	return true
}

func (b *fileBackend) rearm() bool {
	if err := b.arm(); err != nil {
		if !errors.Is(err, fsnotify.ErrClosed) {
			slog.Warn("watch: re-arm file watch", "path", b.path, "err", err)
		}
		return true
	}
	if b.watched != b.parent {
		return true
	}
	if _, err := os.Lstat(b.path); err != nil {
		return true
	}
	return b.send(opCreate)
}

func (b *fileBackend) send(
	o op,
) bool {
	select {
	case b.out <- event{Name: b.path, Op: o}:
		return true
	case <-b.done:
		return false
	}
}

func (b *fileBackend) arm() error {
	for {
		dir, err := b.addNearest()
		if err != nil {
			return err
		}
		if dir == b.watched {
			return nil
		}
		if b.watched != "" {
			dropWatch(b.fsw, b.watched)
		}
		b.watched = dir
		if dir == b.parent {
			return nil
		}
	}
}

func (b *fileBackend) addNearest() (string, error) {
	dir := b.parent
	for {
		err := b.fsw.Add(dir)
		if err == nil {
			return dir, nil
		}
		up := filepath.Dir(dir)
		if !errors.Is(err, fs.ErrNotExist) || up == dir {
			return "", fmt.Errorf("watch %s: %w", dir, err)
		}
		dir = up
	}
}

func watchLimit(
	err error,
) error {
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) || errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("%w: %w", ErrWatchLimit, err)
	}
	return err
}
