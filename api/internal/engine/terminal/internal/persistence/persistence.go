// Package persistence provides atomic read/write helpers for terminal session
// scrollback buffers stored as plain files on disk.
//
// File layout: <dir>/<sessionID>.buf
// Writes are atomic: a unique temp file is written, fsynced, renamed into
// place, and the parent directory is fsynced so the rename survives a crash.
// An interrupted write never corrupts a previously committed .buf file.
package persistence

import (
	"fmt"
	"io"
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

// createTemp is the seam over os.CreateTemp. Production code uses the real
// implementation; tests override it to return a tempFile whose Write/Sync/Close
// fail on demand. It is package-level (not guarded by a lock) and must only be
// reassigned from tests that do not run in parallel.
var createTemp = func(dir, pattern string) (tempFile, error) {
	return os.CreateTemp(dir, pattern)
}

// WriteBuf atomically persists data as <dir>/<sessionID>.buf.
//
// It creates dir if it does not exist. The write is crash-safe:
//  1. A uniquely-named temp file is created inside dir.
//  2. data is written and fsynced to the temp file.
//  3. The temp file is renamed over the destination (atomic on POSIX).
//  4. The parent directory is fsynced so the rename entry is durable.
//
// On any error the temp file is removed; a pre-existing .buf is never
// corrupted by an interrupted call.
func WriteBuf(dir, sessionID string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("persistence: mkdir %s: %w", dir, err)
	}

	tmp, err := createTemp(dir, sessionID+".buf-*")
	if err != nil {
		return fmt.Errorf("persistence: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: write temp: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: sync temp: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: close temp: %w", err)
	}

	dest := filepath.Join(dir, sessionID+".buf")
	if err := os.Rename(tmpName, dest); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("persistence: rename: %w", err)
	}

	// Best-effort: fsync the parent directory so the rename entry is durable
	// across power loss. The rename already succeeded, so the .buf is present
	// and atomic; a dir-fsync failure (e.g. ENOTSUP on some APFS/network
	// volumes) must NOT fail an otherwise-successful write, or it would turn
	// every flush into a spurious error on those filesystems.
	if d, err := os.Open(dir); err == nil { //nolint:gosec // dir is controlled by callers
		_ = d.Sync()
		_ = d.Close()
	}

	return nil
}

// ReadBuf returns the contents of <dir>/<sessionID>.buf.
// If the file does not exist it returns (nil, nil) — a session may have no
// persisted scrollback yet. Real IO errors are wrapped and returned.
func ReadBuf(dir, sessionID string) ([]byte, error) {
	path := filepath.Join(dir, sessionID+".buf")
	data, err := os.ReadFile(path) //nolint:gosec // path controlled by callers
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("persistence: read %s: %w", sessionID, err)
	}
	return data, nil
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
