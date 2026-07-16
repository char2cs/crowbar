// Package mutate performs structural filesystem mutations: create, copy, rename, delete (05 §4).
package mutate

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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

// Copy duplicates the file or directory at sourcePath to destPath, both
// relative to repoPath. File bytes are copied verbatim (io.Copy — no text
// decoding, so binary files stay byte-identical), directories copy
// recursively, and symlinks are recreated as links rather than followed.
// destPath must not already exist (fs.ErrExist) and must not be located
// inside sourcePath — copying a directory into itself would recurse forever.
func Copy(
	repoPath string,
	sourcePath string,
	destPath string,
) error {
	srcFull, err := safepath.Resolve(repoPath, sourcePath)
	if err != nil {
		return fmt.Errorf("mutate: copy source %s: %w", sourcePath, err)
	}
	destFull, err := safepath.Resolve(repoPath, destPath)
	if err != nil {
		return fmt.Errorf("mutate: copy dest %s: %w", destPath, err)
	}
	if destFull == srcFull || strings.HasPrefix(destFull, srcFull+string(filepath.Separator)) {
		return fmt.Errorf(
			"mutate: copy %s: destination %s is inside the source: %w",
			sourcePath, destPath, fs.ErrInvalid,
		)
	}
	info, err := os.Lstat(srcFull)
	if err != nil {
		return fmt.Errorf("mutate: copy %s: %w", sourcePath, err)
	}
	if _, err := os.Lstat(destFull); err == nil {
		return fmt.Errorf("mutate: copy %s: destination %s: %w", sourcePath, destPath, fs.ErrExist)
	}
	if err := os.MkdirAll(filepath.Dir(destFull), 0o700); err != nil {
		return fmt.Errorf("mutate: copy mkdirall %s: %w", destPath, err)
	}
	if err := copyNode(srcFull, destFull, info); err != nil {
		// A recursive copy that fails partway (e.g. an unreadable leaf) leaves a
		// half-populated destination. Remove it so a failed Copy is atomic from
		// the caller's view: the source is never mutated by copyNode, so nothing
		// is lost, but leaving the partial tree behind would both mislead the
		// file view and make an identical retry fail on destination-exists (409).
		// destFull was confirmed absent above, so RemoveAll only discards what
		// this copy created.
		_ = os.RemoveAll(destFull)
		return fmt.Errorf("mutate: copy %s to %s: %w", sourcePath, destPath, err)
	}
	return nil
}

func copyNode(
	srcFull string,
	destFull string,
	info os.FileInfo,
) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return copySymlink(srcFull, destFull)
	}
	if info.IsDir() {
		return copyDir(srcFull, destFull)
	}
	return copyFileBytes(srcFull, destFull)
}

func copySymlink(
	srcFull string,
	destFull string,
) error {
	target, err := os.Readlink(srcFull)
	if err != nil {
		return err
	}
	return os.Symlink(target, destFull)
}

func copyDir(
	srcFull string,
	destFull string,
) error {
	if err := os.Mkdir(destFull, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(srcFull)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyDirEntry(srcFull, destFull, entry); err != nil {
			return err
		}
	}
	return nil
}

func copyDirEntry(
	srcFull string,
	destFull string,
	entry os.DirEntry,
) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	return copyNode(filepath.Join(srcFull, entry.Name()), filepath.Join(destFull, entry.Name()), info)
}

func copyFileBytes(
	srcFull string,
	destFull string,
) error {
	in, err := os.Open(srcFull) //nolint:gosec // G304: srcFull derives from safepath.Resolve output (confined to repoPath)
	if err != nil {
		return err
	}
	defer in.Close()                                                            //nolint:errcheck
	out, err := os.OpenFile(destFull, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // G304: destFull derives from safepath.Resolve output (confined to repoPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
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
	if err := ensureRenameDestFree(oldFull, newFull); err != nil {
		return fmt.Errorf("mutate: rename %s→%s: %w", oldPath, newPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(newFull), 0o700); err != nil {
		return fmt.Errorf("mutate: rename mkdirall %s: %w", newPath, err)
	}
	if err := os.Rename(oldFull, newFull); err != nil {
		return fmt.Errorf("mutate: rename %s→%s: %w", oldPath, newPath, err)
	}
	return nil
}

// ensureRenameDestFree refuses a rename that would clobber an existing, DIFFERENT
// path at the destination. os.Rename silently replaces an occupied destination
// file (and an empty destination directory), so a rename or a drag-drop move onto
// an existing name destroyed the occupant with no error and no undo. Copy already
// refuses an existing destination with fs.ErrExist (mapped to 409); Rename now
// matches, closing the last file-mutation verb that still lost data on collision.
//
// A destination that resolves to the SAME file as the source is allowed: a
// case-only rename on a case-insensitive filesystem (Foo → foo) resolves both
// operands to one inode, which is a legitimate rename rather than a clobber.
func ensureRenameDestFree(
	oldFull string,
	newFull string,
) error {
	destInfo, err := os.Lstat(newFull)
	if err != nil {
		return nil //nolint:nilerr // no destination (ErrNotExist) or an unrelated stat error: let os.Rename decide
	}
	if srcInfo, srcErr := os.Lstat(oldFull); srcErr == nil && os.SameFile(srcInfo, destInfo) {
		return nil
	}
	return fmt.Errorf("destination %s: %w", newFull, fs.ErrExist)
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
