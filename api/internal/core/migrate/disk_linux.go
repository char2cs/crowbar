//go:build linux

package migrate

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// freeBytes reports the bytes an unprivileged process can still allocate on
// the filesystem holding path. Linux counts the free blocks in fragment-size
// units, which can differ from the preferred I/O block size.
func freeBytes(
	path string,
) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("migrate: free space of %s: %w", path, err)
	}
	unit := stat.Frsize
	if unit == 0 {
		unit = stat.Bsize
	}
	if unit <= 0 {
		return 0, fmt.Errorf("migrate: free space of %s: block size %d is unusable", path, unit)
	}
	return stat.Bavail * uint64(unit), nil
}
