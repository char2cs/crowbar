// Package mutate performs structural filesystem mutations: create, rename, delete (05 §4).
package mutate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// CreateFile creates an empty file at filePath. Parent directories are created
// if they do not exist.
func CreateFile(
	repoPath string,
	filePath string,
) error {
	full, err := safepath.Resolve(repoPath, filePath)
	if err != nil {
		return fmt.Errorf("mutate: create %s: %w", filePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return fmt.Errorf("mutate: mkdir %s: %w", filePath, err)
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // containment-validated by safepath.Resolve above
	if err != nil {
		return fmt.Errorf("mutate: create %s: %w", filePath, err)
	}
	return f.Close()
}

// CreateDir creates a directory and all parents.
func CreateDir(
	repoPath string,
	dirPath string,
) error {
	full, err := safepath.Resolve(repoPath, dirPath)
	if err != nil {
		return fmt.Errorf("mutate: mkdir %s: %w", dirPath, err)
	}
	if err := os.MkdirAll(full, 0o700); err != nil {
		return fmt.Errorf("mutate: mkdir %s: %w", dirPath, err)
	}
	return nil
}

// Rename renames or moves a file or directory. Both oldPath and newPath are
// relative to repoPath.
func Rename(
	repoPath string,
	oldPath string,
	newPath string,
) error {
	oldFull, err := safepath.Resolve(repoPath, oldPath)
	if err != nil {
		return fmt.Errorf("mutate: rename old %s: %w", oldPath, err)
	}
	newFull, err := safepath.Resolve(repoPath, newPath)
	if err != nil {
		return fmt.Errorf("mutate: rename new %s: %w", newPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(newFull), 0o700); err != nil {
		return fmt.Errorf("mutate: rename mkdirall %s: %w", newPath, err)
	}
	if err := os.Rename(oldFull, newFull); err != nil {
		return fmt.Errorf("mutate: rename %s→%s: %w", oldPath, newPath, err)
	}
	return nil
}

// Delete removes a file or directory (recursive). filePath is relative to
// repoPath.
func Delete(
	repoPath string,
	filePath string,
) error {
	full, err := safepath.Resolve(repoPath, filePath)
	if err != nil {
		return fmt.Errorf("mutate: delete %s: %w", filePath, err)
	}
	if err := os.RemoveAll(full); err != nil {
		return fmt.Errorf("mutate: delete %s: %w", filePath, err)
	}
	return nil
}
