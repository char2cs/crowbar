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

// errInsufficientDisk is returned when the volume cannot hold a state backup.
var errInsufficientDisk = errors.New("migrate: not enough free disk space for the state backup")

// FreeBytesFunc reports the bytes this process can still allocate on the
// volume holding path.
type FreeBytesFunc func(path string) (uint64, error)

// backupResult describes a finished backup: Path is its directory. An empty
// Path means state/ held nothing to back up.
type backupResult struct {
	Path  string
	Bytes int64
}

// backupState copies state/ (minus the lock file), LAYOUT and the ledger to
// migrate/backups/<runID>, built as <runID>.partial and renamed once complete,
// if the volume has room. The caller holds the lock and has started its run.
func backupState(
	h home.Home,
	runID string,
	layout int,
	free FreeBytesFunc,
) (backupResult, error) {
	if !singleSegment(runID) {
		return backupResult{}, fmt.Errorf("migrate: backup: run id %q is not a single path segment", runID)
	}
	stats, err := measureState(h.State())
	if err != nil {
		return backupResult{}, err
	}
	if stats.files == 0 {
		return backupResult{}, nil
	}
	final := filepath.Join(h.MigrateDir(), backupsDirName, runID)
	if err := requireAbsent(final); err != nil {
		return backupResult{}, err
	}
	if _, err := probeRoom(h.Root(), stats.bytes, free); err != nil {
		return backupResult{}, err
	}
	return writeBackup(h, runID, layout)
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

func singleSegment(
	runID string,
) bool {
	return runID != "." &&
		filepath.IsLocal(runID) &&
		!strings.ContainsAny(runID, `/\`) &&
		!strings.HasSuffix(runID, partialSuffix)
}

// backupRoom is the free space a backup of size bytes needs: the copy and its headroom.
func backupRoom(
	size int64,
) uint64 {
	return uint64(size) + backupHeadroom //nolint:gosec // G115: size sums file lengths, so it is never negative
}

// probeRoom is the free space under root, which must hold a backup of size bytes.
func probeRoom(
	root string,
	size int64,
	free FreeBytesFunc,
) (uint64, error) {
	available, err := free(root)
	if err != nil {
		return 0, fmt.Errorf("migrate: backup: probe free space under %s: %w", root, err)
	}
	required := backupRoom(size)
	if available < required {
		return 0, fmt.Errorf("%w: %d bytes needed, %d free under %s", errInsufficientDisk, required, available, root)
	}
	return available, nil
}

func writeBackup(
	h home.Home,
	runID string,
	layout int,
) (backupResult, error) {
	backups := filepath.Join(h.MigrateDir(), backupsDirName)
	partial := filepath.Join(backups, runID+partialSuffix)
	final := filepath.Join(backups, runID)
	copied, err := fillBackup(h, partial, layout)
	if err != nil {
		return backupResult{}, errors.Join(err, removeTree(partial))
	}
	if err := os.Rename(partial, final); err != nil {
		return backupResult{}, errors.Join(fmt.Errorf("migrate: backup: commit %s: %w", runID, err), removeTree(partial))
	}
	if err := syncDirs(backups, h.MigrateDir(), h.Root()); err != nil {
		return backupResult{}, errors.Join(fmt.Errorf("migrate: backup: make %s durable: %w", runID, err), removeTree(final))
	}
	return backupResult{Path: final, Bytes: copied}, nil
}

// fillBackup writes into dir what a restore puts back, under the names it has
// in the home: state/, LAYOUT and the ledger.
func fillBackup(
	h home.Home,
	dir string,
	layout int,
) (int64, error) {
	copied, err := copyTree(h.State(), filepath.Join(dir, filepath.Base(h.State())))
	if err != nil {
		return 0, err
	}
	if err := stageLayout(filepath.Join(dir, filepath.Base(h.LayoutFile())), layout); err != nil {
		return 0, fmt.Errorf("migrate: backup: %w", err)
	}
	if _, err := copyFileSynced(ledgerPath(h), filepath.Join(dir, ledgerName), 0o600); err != nil {
		return 0, fmt.Errorf("migrate: backup: copy the ledger: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return 0, fmt.Errorf("migrate: backup: flush %s: %w", dir, err)
	}
	return copied, nil
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

func requireAbsent(
	path string,
) error {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("migrate: backup: inspect %s: %w", path, err)
	}
	return fmt.Errorf("migrate: backup: %s already exists: %w", path, fs.ErrExist)
}

func unsupportedEntry(
	path string,
	mode fs.FileMode,
) error {
	return fmt.Errorf("migrate: backup: %s: unsupported %s, a backup holds only regular files and directories", path, entryKind(mode))
}

func entryKind(
	mode fs.FileMode,
) string {
	switch {
	case mode&fs.ModeSymlink != 0:
		return "symlink"
	case mode&fs.ModeSocket != 0:
		return "socket"
	case mode&fs.ModeDevice != 0:
		return "device"
	case mode&fs.ModeNamedPipe != 0:
		return "named pipe"
	default:
		return "irregular file"
	}
}

// resolveRoot follows symlinks in dir itself, so a state directory moved to
// another disk behind a link is scanned and copied as the directory it points
// at. found is false, and root is dir unchanged, when nothing exists there; a
// dangling link is an error.
func resolveRoot(
	dir string,
) (root string, found bool, err error) {
	_, err = os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return dir, false, nil
	}
	if err != nil {
		return dir, false, err
	}
	root, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return dir, false, err
	}
	return root, true, nil
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

// treeStats is what a scan finds. unsupported describes the first entry that is
// neither a directory nor a regular file.
type treeStats struct {
	files       int
	bytes       int64
	unsupported error
}

func scanTree(
	dir string,
) (treeStats, error) {
	var stats treeStats
	root, found, err := resolveRoot(dir)
	if err != nil || !found {
		return stats, err
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		return stats.visit(root, path, entry, walkErr)
	})
	return stats, err
}

// measureState scans dir for what a backup would copy. An entry a backup cannot
// take is an error, so a backup that is due and cannot be made fails here.
func measureState(
	dir string,
) (treeStats, error) {
	stats, err := scanTree(dir)
	if err != nil {
		return treeStats{}, fmt.Errorf("migrate: backup: measure %s: %w", dir, err)
	}
	return stats, stats.unsupported
}

func (s *treeStats) visit(
	root string,
	path string,
	entry fs.DirEntry,
	walkErr error,
) error {
	if walkErr != nil {
		return walkErr
	}
	if entry.IsDir() || isHomeLock(root, path, entry) {
		return nil
	}
	if !entry.Type().IsRegular() {
		if s.unsupported == nil {
			s.unsupported = unsupportedEntry(path, entry.Type())
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
	root, _, err := resolveRoot(src)
	if err != nil {
		return 0, fmt.Errorf("migrate: backup: resolve %s: %w", src, err)
	}
	copying := &treeCopy{src: root, dst: dst}
	if err := filepath.WalkDir(root, copying.visit); err != nil {
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
		return unsupportedEntry(path, entry.Type())
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
