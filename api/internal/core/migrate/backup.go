package migrate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

const (
	// backupHeadroom is the free space that must remain beyond the copy itself,
	// so a backup never leaves the disk too full for the daemon to run.
	backupHeadroom = 64 << 20
	backupsDirName = "backups"
	partialSuffix  = ".partial"
	homeLockName   = ".lock"
)

// ErrInsufficientDisk is returned when the volume cannot hold a state backup.
var ErrInsufficientDisk = errors.New("migrate: not enough free disk space for the state backup")

// FreeBytesFunc reports the bytes this process can still allocate on the
// volume holding path.
type FreeBytesFunc func(path string) (uint64, error)

// backupResult describes a finished backup. An empty Path means state/ held
// nothing to back up.
type backupResult struct {
	Path  string
	Bytes int64
}

// backupState copies state/ to migrate/backups/<runID>/state. The home lock
// file is left out; every other regular file, however small, is copied. The
// copy is built under <runID>.partial and renamed into place once complete, so
// a failure leaves neither directory. Nothing is created unless the volume has
// room for the copy plus backupHeadroom.
func backupState(
	h home.Home,
	runID string,
	free FreeBytesFunc,
) (backupResult, error) {
	if !singleSegment(runID) {
		return backupResult{}, fmt.Errorf("migrate: backup: run id %q is not a single path segment", runID)
	}
	stats, err := scanTree(h.State())
	if err != nil {
		return backupResult{}, fmt.Errorf("migrate: backup: measure %s: %w", h.State(), err)
	}
	if stats.unsupported != "" {
		return backupResult{}, unsupportedEntry(stats.unsupported)
	}
	if stats.files == 0 {
		return backupResult{}, nil
	}
	if err := requireFreeSpace(h.Root(), stats.bytes, free); err != nil {
		return backupResult{}, err
	}
	return writeBackup(h, runID)
}

// cleanPartialBackups removes the half-written backups a crashed run left in
// migrate/backups. An absent directory is not created.
func cleanPartialBackups(
	h home.Home,
) error {
	dir := filepath.Join(h.MigrateDir(), backupsDirName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("migrate: backup: list %s: %w", dir, err)
	}
	var failures []error
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), partialSuffix) {
			failures = append(failures, removeTree(filepath.Join(dir, entry.Name())))
		}
	}
	return errors.Join(failures...)
}

// treeSize is the total bytes of the regular files under dir, not counting the
// home lock file. An absent dir has size zero.
func treeSize(
	dir string,
) (int64, error) {
	stats, err := scanTree(dir)
	return stats.bytes, err
}

func singleSegment(
	runID string,
) bool {
	return runID != "." &&
		filepath.IsLocal(runID) &&
		!strings.ContainsAny(runID, `/\`) &&
		!strings.HasSuffix(runID, partialSuffix)
}

func requireFreeSpace(
	root string,
	size int64,
	free FreeBytesFunc,
) error {
	available, err := free(root)
	if err != nil {
		return fmt.Errorf("migrate: backup: probe free space under %s: %w", root, err)
	}
	required := uint64(size) + backupHeadroom //nolint:gosec // G115: size sums file lengths, so it is never negative
	if available < required {
		return fmt.Errorf("%w: %d bytes needed, %d free under %s", ErrInsufficientDisk, required, available, root)
	}
	return nil
}

func writeBackup(
	h home.Home,
	runID string,
) (backupResult, error) {
	backups := filepath.Join(h.MigrateDir(), backupsDirName)
	partial := filepath.Join(backups, runID+partialSuffix)
	final := filepath.Join(backups, runID)
	copied, err := copyTree(h.State(), filepath.Join(partial, "state"))
	if err != nil {
		return backupResult{}, errors.Join(err, removeTree(partial))
	}
	if err := os.Rename(partial, final); err != nil {
		return backupResult{}, errors.Join(fmt.Errorf("migrate: backup: commit %s: %w", runID, err), removeTree(partial))
	}
	if err := syncDirs(backups, h.MigrateDir(), h.Root()); err != nil {
		return backupResult{}, errors.Join(fmt.Errorf("migrate: backup: make %s durable: %w", runID, err), removeTree(final))
	}
	return backupResult{Path: filepath.Join(final, "state"), Bytes: copied}, nil
}

func removeTree(
	path string,
) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("migrate: backup: remove %s: %w", path, err)
	}
	return nil
}

func syncDirs(
	dirs ...string,
) error {
	for _, dir := range dirs {
		if err := syncDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func unsupportedEntry(
	path string,
) error {
	return fmt.Errorf("migrate: backup: %s is neither a regular file nor a directory, the only entries a backup can hold", path)
}

// isHomeLock matches only <root>/.lock: a file of that name deeper in the tree
// is user data.
func isHomeLock(
	root string,
	path string,
	entry fs.DirEntry,
) bool {
	return path == filepath.Join(root, homeLockName) && !entry.IsDir()
}

// treeStats is what a scan finds. unsupported is the first entry that is
// neither a directory nor a regular file.
type treeStats struct {
	files       int
	bytes       int64
	unsupported string
}

func scanTree(
	dir string,
) (treeStats, error) {
	var stats treeStats
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		return stats.visit(dir, path, entry, walkErr)
	})
	return stats, err
}

func (s *treeStats) visit(
	root string,
	path string,
	entry fs.DirEntry,
	walkErr error,
) error {
	if errors.Is(walkErr, fs.ErrNotExist) && path == root {
		return nil
	}
	if walkErr != nil {
		return walkErr
	}
	if entry.IsDir() || isHomeLock(root, path, entry) {
		return nil
	}
	if !entry.Type().IsRegular() {
		if s.unsupported == "" {
			s.unsupported = path
		}
		return nil
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	s.files++
	s.bytes += info.Size()
	return nil
}

// treeCopy mirrors src into dst. dirs lists every directory it created, so
// their entries can be flushed once the walk is done.
type treeCopy struct {
	src   string
	dst   string
	bytes int64
	dirs  []string
}

func copyTree(
	src string,
	dst string,
) (int64, error) {
	copying := &treeCopy{src: src, dst: dst}
	if err := filepath.WalkDir(src, copying.visit); err != nil {
		return 0, fmt.Errorf("migrate: backup: copy %s: %w", src, err)
	}
	if err := syncDirs(append(copying.dirs, filepath.Dir(dst))...); err != nil {
		return 0, fmt.Errorf("migrate: backup: flush %s: %w", dst, err)
	}
	return copying.bytes, nil
}

func (c *treeCopy) visit(
	path string,
	entry fs.DirEntry,
	walkErr error,
) error {
	if walkErr != nil {
		return walkErr
	}
	if isHomeLock(c.src, path, entry) {
		return nil
	}
	rel, err := filepath.Rel(c.src, path)
	if err != nil {
		return err
	}
	target := filepath.Join(c.dst, rel)
	switch {
	case entry.IsDir():
		c.dirs = append(c.dirs, target)
		return os.MkdirAll(target, 0o700)
	case entry.Type().IsRegular():
		return c.copyFile(path, target, entry)
	default:
		return unsupportedEntry(path)
	}
}

func (c *treeCopy) copyFile(
	src string,
	dst string,
	entry fs.DirEntry,
) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	written, err := copyFileSynced(src, dst, info.Mode().Perm())
	if err != nil {
		return err
	}
	c.bytes += written
	return nil
}

// copyFileSynced copies src to a new file dst and flushes it to disk. The file
// is created owner-writable and given mode afterwards, so neither a read-only
// source nor the umask changes the copy's mode.
func copyFileSynced(
	src string,
	dst string,
	mode fs.FileMode,
) (int64, error) {
	in, err := os.Open(src) //nolint:gosec // G304: src comes from walking the home's own state directory
	if err != nil {
		return 0, err
	}
	written, err := writeSynced(in, dst, mode)
	err = errors.Join(err, in.Close())
	if err != nil {
		return 0, err
	}
	return written, nil
}

func writeSynced(
	in io.Reader,
	dst string,
	mode fs.FileMode,
) (int64, error) {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: dst is inside the backup directory this run just created
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(out, in)
	if err == nil {
		err = out.Chmod(mode)
	}
	if err == nil {
		err = out.Sync()
	}
	return written, errors.Join(err, out.Close())
}
