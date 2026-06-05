// Package mutate performs structural filesystem mutations: create, rename, delete (05 §4).
package mutate

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateFile creates an empty file at filePath. Parent directories are created
// if they do not exist.
func CreateFile(
	repoPath string,
	filePath string,
) error {
	full := filepath.Join(repoPath, filePath)
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		return fmt.Errorf("mutate: mkdir %s: %w", filePath, err)
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL, 0600)
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
	full := filepath.Join(repoPath, dirPath)
	if err := os.MkdirAll(full, 0700); err != nil {
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
	oldFull := filepath.Join(repoPath, oldPath)
	newFull := filepath.Join(repoPath, newPath)
	if err := os.MkdirAll(filepath.Dir(newFull), 0700); err != nil {
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
	full := filepath.Join(repoPath, filePath)
	if err := os.RemoveAll(full); err != nil {
		return fmt.Errorf("mutate: delete %s: %w", filePath, err)
	}
	return nil
}
