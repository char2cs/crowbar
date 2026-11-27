package persistence

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type tempFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
}

var createTemp = func(dir, pattern string) (tempFile, error) {
	return os.CreateTemp(dir, pattern)
}

func WriteBuf(dir, sessionID string, data []byte) (err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
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

func closeAfter(tmp tempFile, failure error) error {
	if err := tmp.Close(); err != nil {
		return errors.Join(failure, fmt.Errorf("persistence: close temp: %w", err))
	}
	return failure
}

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
