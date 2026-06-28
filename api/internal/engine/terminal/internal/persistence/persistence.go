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
	"os"
	"path/filepath"
)

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

	tmp, err := os.CreateTemp(dir, sessionID+".buf-*")
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

	// Fsync the parent directory so the directory entry for the rename is
	// durable across a power loss or OS crash.
	d, err := os.Open(dir) //nolint:gosec // dir is controlled by callers
	if err != nil {
		return fmt.Errorf("persistence: open dir for sync: %w", err)
	}
	syncErr := d.Sync()
	_ = d.Close()
	if syncErr != nil {
		// The rename already succeeded; the file is present. Treat a
		// dir-sync failure as a non-fatal durability concern rather than
		// data loss (some filesystems reject fsync on directories).
		return fmt.Errorf("persistence: sync dir: %w", syncErr)
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
