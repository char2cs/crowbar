// Package persistence provides atomic read/write helpers for terminal scrollback
// buffers stored as <dir>/<sessionID>.buf. A write goes through an fsynced temp file
// and rename plus a parent-dir fsync, so a crash never corrupts a committed .buf.
package persistence

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// tempFile is the narrow subset of *os.File that WriteBuf uses for its staging
// file. It exists only as a file-local seam so tests can inject Write/Sync/Close
// failures that a real filesystem cannot be coerced into producing reliably.
type tempFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
}

// createTemp is the seam over os.CreateTemp that tests replace to fail writes on
// demand; it is unguarded, so only non-parallel tests may reassign it.
var createTemp = func(dir, pattern string) (tempFile, error) {
	return os.CreateTemp(dir, pattern)
}

// WriteBuf atomically persists data as <dir>/<sessionID>.buf, creating dir if needed:
// temp file, fsync, rename, fsync of the parent. On error the temp file is removed (a
// failed removal is reported too), and an existing .buf is never corrupted.
func WriteBuf(dir, sessionID string, data []byte) (err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: the terminal-buffer dir lives under the user's own ~/.crowbar home; 0755 intentional and matches the rest of the tree.
		return fmt.Errorf("persistence: mkdir %s: %w", dir, err)
	}

	tmp, err := createTemp(dir, sessionID+".buf-*")
	if err != nil {
		return fmt.Errorf("persistence: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err == nil {
			return
		}
		if rmErr := os.Remove(tmpName); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("persistence: remove temp: %w", rmErr))
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return closeAfter(tmp, fmt.Errorf("persistence: write temp: %w", err))
	}

	if err := tmp.Sync(); err != nil {
		return closeAfter(tmp, fmt.Errorf("persistence: sync temp: %w", err))
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("persistence: close temp: %w", err)
	}

	dest := filepath.Join(dir, sessionID+".buf")
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("persistence: rename: %w", err)
	}

	syncDir(dir)
	return nil
}

// closeAfter closes tmp after failure, reporting a close error alongside it.
func closeAfter(tmp tempFile, failure error) error {
	if err := tmp.Close(); err != nil {
		return errors.Join(failure, fmt.Errorf("persistence: close temp: %w", err))
	}
	return failure
}

// syncDir fsyncs dir so the completed rename survives power loss. Its failure (ENOTSUP on
// some APFS/network volumes) is logged, not returned: the .buf is already in place and
// returning it would fail every flush there.
func syncDir(dir string) {
	d, err := os.Open(dir) //nolint:gosec // dir is controlled by callers
	if err != nil {
		slog.Warn("persistence: open dir to sync", "dir", dir, "err", err)
		return
	}
	if err := errors.Join(d.Sync(), d.Close()); err != nil {
		slog.Warn("persistence: sync dir", "dir", dir, "err", err)
	}
}

// DeleteBuf removes <dir>/<sessionID>.buf.
// A missing file is not an error.
func DeleteBuf(dir, sessionID string) error {
	path := filepath.Join(dir, sessionID+".buf")
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("persistence: delete %s: %w", sessionID, err)
	}
	return nil
}
