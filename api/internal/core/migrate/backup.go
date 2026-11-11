package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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

// backupResult describes a finished backup: Path is its archive, Bytes what
// it holds and Packed its size on disk. An empty Path means there was nothing
// to back up.
type backupResult struct {
	Path   string
	Bytes  int64
	Packed int64
}

// backupState archives state/ (minus the lock file), LAYOUT, the ledger and the
// paths extra matches under the home to migrate/backups/<runID>.tar.gz, built as
// a .partial and renamed once it reads back as written, if the volume has room.
// The caller holds the lock and has started its run.
func backupState(
	h home.Home,
	runID string,
	layout int,
	extra []string,
	free FreeBytesFunc,
) (backupResult, error) {
	if !singleSegment(runID) {
		return backupResult{}, fmt.Errorf("migrate: backup: run id %q is not a single path segment", runID)
	}
	stats, err := measureBackup(h, extra)
	if err != nil {
		return backupResult{}, err
	}
	if stats.files == 0 {
		return backupResult{}, nil
	}
	final := backupPath(h, runID)
	if err := requireAbsent(final); err != nil {
		return backupResult{}, err
	}
	if _, err := probeRoom(h.Root(), stats.bytes, free); err != nil {
		return backupResult{}, err
	}
	return writeBackup(h, runID, layout, extra)
}

// backupPath is where run's backup lives once whole.
func backupPath(
	h home.Home,
	runID string,
) string {
	return filepath.Join(h.MigrateDir(), backupsDirName, runID+backupExt)
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
	extra []string,
) (backupResult, error) {
	backups := filepath.Join(h.MigrateDir(), backupsDirName)
	final := backupPath(h, runID)
	partial := final + partialSuffix
	if err := os.MkdirAll(backups, 0o700); err != nil {
		return backupResult{}, fmt.Errorf("migrate: backup: %w", err)
	}
	a, err := writeArchive(partial, func(a *archiver) error { return fillBackup(h, a, layout, extra) })
	if err == nil {
		err = verifyArchive(partial, a.sums)
	}
	if err != nil {
		return backupResult{}, errors.Join(fmt.Errorf("migrate: backup: %w", err), removeTree(partial))
	}
	if err := os.Rename(partial, final); err != nil {
		return backupResult{}, errors.Join(fmt.Errorf("migrate: backup: commit %s: %w", runID, err), removeTree(partial))
	}
	if err := syncDirs(backups, h.MigrateDir(), h.Root()); err != nil {
		return backupResult{}, errors.Join(fmt.Errorf("migrate: backup: make %s durable: %w", runID, err), removeTree(final))
	}
	info, err := os.Stat(final)
	if err != nil {
		return backupResult{}, fmt.Errorf("migrate: backup: %w", err)
	}
	return backupResult{Path: final, Bytes: a.bytes, Packed: info.Size()}, nil
}

// fillBackup writes what a restore puts back, under the names it has in the
// home: state/, LAYOUT, the ledger and whatever extra matches.
func fillBackup(
	h home.Home,
	a *archiver,
	layout int,
	extra []string,
) error {
	if err := a.addTree(h.State(), filepath.Base(h.State()), isHomeLock); err != nil {
		return err
	}
	if err := a.addBytes(filepath.Base(h.LayoutFile()), []byte(strconv.Itoa(layout)+"\n")); err != nil {
		return err
	}
	ledger := ledgerPath(h)
	info, err := os.Stat(ledger)
	if err != nil {
		return fmt.Errorf("copy the ledger: %w", err)
	}
	if err := a.addFile(homeRel(h, ledger), ledger, info); err != nil {
		return err
	}
	matches, err := extraMatches(h, extra)
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := a.addTree(path, homeRel(h, path), noSkip); err != nil {
			return err
		}
	}
	return nil
}

func noSkip(string, string, fs.DirEntry) bool { return false }

// homeRel is path as a backup names it: slash-separated, under the home.
func homeRel(
	h home.Home,
	path string,
) string {
	rel, err := filepath.Rel(h.Root(), path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// extraMatches expands the home-relative patterns a plan asked to back up.
// A pattern that could reach outside the home or into state/ is refused.
func extraMatches(
	h home.Home,
	patterns []string,
) ([]string, error) {
	var out []string
	for _, pattern := range patterns {
		local := filepath.FromSlash(pattern)
		if !filepath.IsLocal(local) || strings.HasPrefix(pattern, filepath.Base(h.State())+"/") {
			return nil, fmt.Errorf("migrate: backup: %q is not a path under the home outside state/", pattern)
		}
		matches, err := filepath.Glob(filepath.Join(h.Root(), local))
		if err != nil {
			return nil, fmt.Errorf("migrate: backup: %q: %w", pattern, err)
		}
		out = append(out, matches...)
	}
	return out, nil
}

// measureBackup is what a backup would hold: state/ and the extra matches.
func measureBackup(
	h home.Home,
	extra []string,
) (treeStats, error) {
	stats, err := measureState(h.State())
	if err != nil || stats.files == 0 {
		return stats, err
	}
	matches, err := extraMatches(h, extra)
	if err != nil {
		return treeStats{}, err
	}
	for _, path := range matches {
		more, err := measureState(path)
		if err != nil {
			return treeStats{}, err
		}
		stats.files += more.files
		stats.bytes += more.bytes
	}
	return stats, nil
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

// resolveRoot follows symlinks in dir itself, so a state dir moved behind a link is
// copied as its target. found is false (root is dir) when nothing exists there; a
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
